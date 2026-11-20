// Package relate joins saved line candidates using the existing C call graph.
package relate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/noperator/slice/pkg/codeql"
	"github.com/noperator/slice/pkg/parser"
)

type Relationship struct {
	Relationship string            `json:"relationship"`
	Match        string            `json:"match"`
	Left         json.RawMessage   `json:"left"`
	Right        json.RawMessage   `json:"right"`
	CallChain    []json.RawMessage `json:"call_chain"`
}

type Result struct {
	Relationships []Relationship `json:"relationships"`
}

func readCollection(filename, collection string) ([]json.RawMessage, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	raw, ok := document[collection]
	if !ok || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '[' {
		return nil, fmt.Errorf("%s: expected %q array", filename, collection)
	}
	var records []json.RawMessage
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	return records, nil
}

type candidate struct {
	raw        json.RawMessage
	functionID string
}

func readCandidates(filename string, functions []parser.Function) ([]candidate, int, error) {
	records, err := readCollection(filename, "lines")
	if err != nil {
		return nil, 0, err
	}
	candidates := make([]candidate, 0, len(records))
	unresolved := 0
	for i, raw := range records {
		var line struct {
			ID         *string `json:"id"`
			File       *string `json:"file"`
			Start      *int    `json:"start"`
			End        *int    `json:"end"`
			Definition *string `json:"def"`
		}
		err := json.Unmarshal(raw, &line)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: record %d: %w", filename, i+1, err)
		}
		if line.ID == nil || line.File == nil || *line.File == "" || line.Start == nil || line.End == nil ||
			*line.Start <= 0 || *line.Start != *line.End || line.Definition == nil || *line.ID != fmt.Sprintf("%s:%d", *line.File, *line.Start) {
			return nil, 0, fmt.Errorf("%s: record %d: expected a line with file, positive equal start/end, matching id, and string def", filename, i+1)
		}
		id := ""
		for _, function := range functions {
			if function.Filename == *line.File && function.StartLine <= *line.Start && *line.Start <= function.EndLine {
				id = function.ID
				break
			}
		}
		if id == "" {
			unresolved++
			continue
		}
		candidates = append(candidates, candidate{raw: raw, functionID: id})
	}
	return candidates, unresolved, nil
}

// Run reads only saved JSON; raw records retain unknown fields and exact numbers.
func Run(parsedFile, leftFile, rightFile, relationship string, depth int, diagnostics io.Writer) (*Result, error) {
	if relationship != "call" {
		return nil, fmt.Errorf("unsupported relationship %q (expected call)", relationship)
	}
	if depth < 0 {
		return nil, fmt.Errorf("call-depth must be nonnegative")
	}
	records, err := readCollection(parsedFile, "functions")
	if err != nil {
		return nil, err
	}
	functions := make([]parser.Function, len(records))
	originals := make(map[string]json.RawMessage, len(records))
	for i, raw := range records {
		function := &functions[i]
		if err := json.Unmarshal(raw, function); err != nil {
			return nil, fmt.Errorf("%s: record %d: %w", parsedFile, i+1, err)
		}
		if function.ID == "" || function.Filename == "" || function.StartLine <= 0 || function.EndLine < function.StartLine {
			return nil, fmt.Errorf("%s: record %d: invalid function identity or source range", parsedFile, i+1)
		}
		originals[function.ID] = raw
	}
	left, leftUnresolved, err := readCandidates(leftFile, functions)
	if err != nil {
		return nil, err
	}
	right, rightUnresolved, err := readCandidates(rightFile, functions)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(diagnostics, "Unresolved candidates: left=%d right=%d\n", leftUnresolved, rightUnresolved)
	graph := codeql.BuildCallGraph(functions)
	type pair struct{ from, to string }
	cache := make(map[pair][]string)
	callChain := func(from, to string) []string {
		key := pair{from, to}
		if chain, ok := cache[key]; ok {
			return chain
		}
		chain := graph.ShortestCallChain(from, to, depth)
		cache[key] = chain
		return chain
	}
	result := &Result{Relationships: []Relationship{}}
	for _, l := range left {
		for _, r := range right {
			match := "same_function"
			chain := []string{l.functionID}
			if l.functionID != r.functionID {
				match, chain = "left_reaches_right", callChain(l.functionID, r.functionID)
				if len(chain) == 0 {
					match, chain = "right_reaches_left", callChain(r.functionID, l.functionID)
				}
				if len(chain) == 0 {
					continue
				}
			}
			fullChain := make([]json.RawMessage, len(chain))
			for i, id := range chain {
				fullChain[i] = originals[id]
			}
			result.Relationships = append(result.Relationships, Relationship{
				Relationship: "call", Match: match, Left: l.raw, Right: r.raw, CallChain: fullChain,
			})
		}
	}
	return result, nil
}
