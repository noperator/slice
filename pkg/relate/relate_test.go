package relate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/noperator/slice/pkg/parser"
)

// The saved-JSON join must respect direction/depth and preserve records without numeric rounding.
func TestCallRelationships(t *testing.T) {
	parsed, err := parser.ExtractDirectory("../../testdata/calls", true, false)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	parsedFile := filepath.Join(dir, "parsed.json")
	data, _ := json.Marshal(parsed)
	if err := os.WriteFile(parsedFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	functions := *parsed.Functions
	candidateFor := func(index int) string {
		f := functions[index]
		return fmt.Sprintf(`{"id":%q,"file":%q,"start":%d,"end":%d,"def":"","metadata":{"integer":9007199254740993}}`, fmt.Sprintf("%s:%d", f.Filename, f.StartLine), f.Filename, f.StartLine, f.StartLine)
	}
	// Fixture indices: leaf, middle, sibling, entry, duplicate in a.c, duplicate in b.h.
	for _, tc := range []struct {
		name               string
		left, right, depth int
		match              string
		chain              []int
	}{
		{"same", 0, 0, 0, "same_function", []int{0}},
		{"forward", 3, 1, 1, "left_reaches_right", []int{3, 1}},
		{"reverse", 1, 3, 1, "right_reaches_left", []int{3, 1}},
		{"two edges", 3, 0, 2, "left_reaches_right", []int{3, 1, 0}},
		{"depth limit", 3, 0, 1, "", nil},
		{"zero depth", 3, 1, 0, "", nil},
		{"cycle precedence", 0, 1, 3, "left_reaches_right", []int{0, 1}},
		{"common caller", 1, 2, 3, "", nil},
		{"unrelated cycle", 0, 4, 100, "", nil},
		{"duplicate names", 4, 5, 3, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			left, right := filepath.Join(dir, "left.json"), filepath.Join(dir, "right.json")
			leftRecord, rightRecord := candidateFor(tc.left), candidateFor(tc.right)
			for path, record := range map[string]string{left: leftRecord, right: rightRecord} {
				if err := os.WriteFile(path, []byte(`{"lines":[`+record+`]}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var diagnostics bytes.Buffer
			result, err := Run(parsedFile, left, right, "call", tc.depth, &diagnostics)
			if err != nil {
				t.Fatal(err)
			}
			if tc.match == "" {
				if len(result.Relationships) != 0 {
					t.Fatalf("unexpected match: %+v", result)
				}
				return
			}
			if len(result.Relationships) != 1 {
				t.Fatalf("got %d matches", len(result.Relationships))
			}
			r := result.Relationships[0]
			if r.Match != tc.match || string(r.Left) != leftRecord || string(r.Right) != rightRecord {
				t.Fatalf("incorrect relationship: %+v", r)
			}
			if len(r.CallChain) != len(tc.chain) {
				t.Fatalf("wrong chain: %s", r.CallChain)
			}
			for i, index := range tc.chain {
				expected, _ := json.Marshal(functions[index])
				if !bytes.Equal(r.CallChain[i], expected) {
					t.Fatalf("function record changed: %s", r.CallChain[i])
				}
			}
			encoded, _ := json.Marshal(result)
			if !bytes.Contains(encoded, []byte("9007199254740993")) {
				t.Fatal("metadata rounded")
			}
		})
	}
}

// Bad saved inputs need actionable record errors; unresolved records count independently of the other side.
func TestInputValidationAndUnresolved(t *testing.T) {
	dir := t.TempDir()
	parsed, left, right := filepath.Join(dir, "parsed.json"), filepath.Join(dir, "left.json"), filepath.Join(dir, "right.json")
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(parsed, `{"functions":[],"lines":[]}`)
	write(right, `{"lines":[]}`)
	unresolved := `{"id":"missing.c:1","file":"missing.c","start":1,"end":1,"def":""}`
	write(left, `{"lines":[`+unresolved+`,`+unresolved+`]}`)
	var diagnostics bytes.Buffer
	result, err := Run(parsed, left, right, "call", 3, &diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	if string(encoded) != `{"relationships":[]}` || !strings.Contains(diagnostics.String(), "left=2 right=0") {
		t.Fatalf("%s; %s", encoded, &diagnostics)
	}
	write(left, `{"lines":[]}`)
	write(right, `{"lines":[`+unresolved+`]}`)
	diagnostics.Reset()
	if _, err := Run(parsed, left, right, "call", 3, &diagnostics); err != nil || !strings.Contains(diagnostics.String(), "left=0 right=1") {
		t.Fatalf("%v; %s", err, &diagnostics)
	}
	write(right, `{"lines":[]}`)
	for _, bad := range []string{`{`, `null`, `[]`, `{}`, `{"lines":null}`, `{"lines":{}}`, `{"lines":[]} {}`} {
		write(left, bad)
		if _, err := Run(parsed, left, right, "call", 3, &diagnostics); err == nil || !strings.Contains(err.Error(), left) {
			t.Fatalf("accepted %s: %v", bad, err)
		}
	}
	for _, bad := range []string{`null`, `{}`, `{"id":"x:1","file":"x","start":1,"end":1}`, `{"id":"x:1","file":"x","start":1,"end":1,"def":null}`, `{"id":"x:1","file":"x","start":1.5,"end":1,"def":""}`, `{"id":"x:1","file":"x","start":1,"end":2,"def":""}`, `{"id":"wrong","file":"x","start":1,"end":1,"def":""}`, `{"id":":1","file":"","start":1,"end":1,"def":""}`, `{"id":"x:0","file":"x","start":0,"end":0,"def":""}`} {
		write(left, `{"lines":[`+bad+`]}`)
		if _, err := Run(parsed, left, right, "call", 3, &diagnostics); err == nil || !strings.Contains(err.Error(), left+": record 1:") {
			t.Fatalf("accepted %s: %v", bad, err)
		}
	}
	write(left, `{"lines":[]}`)
	for _, bad := range []string{`{}`, `{"functions":null}`, `{"functions":[null]}`, `{"functions":[{"start":"bad"}]}`} {
		write(parsed, bad)
		if _, err := Run(parsed, left, right, "call", 3, &diagnostics); err == nil || !strings.Contains(err.Error(), parsed) {
			t.Fatalf("accepted %s: %v", bad, err)
		}
	}
	write(parsed, `{"functions":[]}`)
	for _, args := range []struct {
		relationship string
		depth        int
	}{{"other", 3}, {"call", -1}} {
		if _, err := Run(parsed, left, right, args.relationship, args.depth, &diagnostics); err == nil {
			t.Fatal("accepted invalid options")
		}
	}
}

// Multiple candidates in each function must retain left-then-right input order.
func TestPairOrder(t *testing.T) {
	dir := t.TempDir()
	parsed, left, right := filepath.Join(dir, "parsed.json"), filepath.Join(dir, "left.json"), filepath.Join(dir, "right.json")
	inputs := map[string]string{
		parsed: `{"functions":[{"id":"x:1:f","file":"x","start":1,"end":3,"def":"complete","extra":9007199254740993}]}`,
		left:   `{"lines":[{"id":"x:2","file":"x","start":2,"end":2,"def":"first"},{"id":"x:1","file":"x","start":1,"end":1,"def":"second"}]}`,
		right:  `{"lines":[{"id":"x:3","file":"x","start":3,"end":3,"def":"third"},{"id":"x:1","file":"x","start":1,"end":1,"def":"fourth"}]}`,
	}
	for path, content := range inputs {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Run(parsed, left, right, "call", 0, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, r := range result.Relationships {
		var l, rr parser.Line
		json.Unmarshal(r.Left, &l)
		json.Unmarshal(r.Right, &rr)
		got = append(got, l.Definition+"/"+rr.Definition)
		if !bytes.Contains(r.CallChain[0], []byte(`"extra":9007199254740993`)) {
			t.Fatal("lost function metadata")
		}
	}
	if !reflect.DeepEqual(got, []string{"first/third", "first/fourth", "second/third", "second/fourth"}) {
		t.Fatal(got)
	}
}
