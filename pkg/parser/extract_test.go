package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Physical lines must retain source positions and whitespace even outside functions.
func TestPhysicalLines(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          []string
	}{
		{"empty", "", []string{}},
		{"mixed", "// comment\r\n\n \t \n{\n;\n}\nlast\r", []string{"// comment", "", " \t ", "{", ";", "}", "last\r"}},
		{"crlf", "\r\ntext\r\r\n", []string{"", "text\r"}},
		{"final newline", "one\n", []string{"one"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "lines.c")
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := ExtractDirectory(dir, false, true)
			if err != nil {
				t.Fatal(err)
			}
			got := []string{}
			for i, line := range *result.Lines {
				if line.Start != i+1 || line.End != i+1 || line.Filename != path || line.ID != fmt.Sprintf("%s:%d", path, i+1) {
					t.Fatalf("bad position: %+v", line)
				}
				got = append(got, line.Definition)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

// Selective extraction must preserve the established function schema and omit unrequested collections.
func TestExtractionSelections(t *testing.T) {
	dir := "../../testdata/calls"
	existing, err := GetCachedAnalysisResult(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range [][2]bool{{true, false}, {false, true}, {true, true}} {
		result, err := ExtractDirectory(dir, selection[0], selection[1])
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]json.RawMessage
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		if (document["functions"] != nil) != selection[0] || (document["lines"] != nil) != selection[1] {
			t.Fatalf("unexpected collections: %s", data)
		}
		if selection[0] && !reflect.DeepEqual(*result.Functions, existing.Functions) {
			t.Fatal("function records changed")
		}
		if selection[1] {
			previous := ""
			for _, line := range *result.Lines {
				if line.Filename < previous {
					t.Fatal("files out of order")
				}
				previous = line.Filename
			}
		}
	}
	function := existing.Functions[0]
	if function.ID != "../../testdata/calls/a.c:2:leaf" || function.Name != "leaf" || function.StartLine != 2 || function.EndLine != 4 || function.Definition != "void leaf(void) {\n    middle();\n}" || len(function.Callees) != 1 || function.Callees[0].Name != "middle" {
		t.Fatalf("incompatible function: %+v", function)
	}
	empty, err := ExtractDirectory(t.TempDir(), true, true)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(empty)
	if string(data) != `{"functions":[],"lines":[]}` {
		t.Fatalf("empty output: %s", data)
	}
	if _, err := ExtractDirectory(filepath.Join(dir, "a.c"), true, true); err == nil {
		t.Fatal("accepted a file as directory")
	}
}

// Directory walk order can differ from global file-path order; nonregular C entries must be skipped.
func TestLineFileOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "a"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a/z.c", "a.c", "ignore.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("line\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "a.c"), filepath.Join(dir, "link.c")); err != nil {
		t.Fatal(err)
	}
	result, err := ExtractDirectory(dir, false, true)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, line := range *result.Lines {
		got = append(got, line.Filename)
	}
	want := []string{filepath.Join(dir, "a.c"), filepath.Join(dir, "a/z.c")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
