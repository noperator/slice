package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
)

// Keep the backward-compatible functions-only default while allowing explicit line selection.
func TestParseSelection(t *testing.T) {
	for _, tc := range []struct {
		name             string
		flags            []string
		functions, lines bool
	}{
		{"default", nil, true, false},
		{"functions", []string{"--functions"}, true, false},
		{"lines", []string{"--lines"}, false, true},
		{"both", []string{"--functions", "--lines"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().Bool("functions", false, "")
			cmd.Flags().Bool("lines", false, "")
			if err := cmd.Flags().Parse(tc.flags); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			cmd.SetOut(&output)
			if err := parseCmd.RunE(cmd, []string{"../../testdata/calls"}); err != nil {
				t.Fatal(err)
			}
			var document map[string]json.RawMessage
			if err := json.Unmarshal(output.Bytes(), &document); err != nil {
				t.Fatal(err)
			}
			if (document["functions"] != nil) != tc.functions || (document["lines"] != nil) != tc.lines {
				t.Fatalf("unexpected collections: %s", &output)
			}
		})
	}
}
