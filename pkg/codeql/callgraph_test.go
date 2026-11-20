package codeql

import (
	"reflect"
	"testing"

	"github.com/noperator/slice/pkg/parser"
)

// Equal shortest call chains must be deterministic regardless of callee insertion order.
func TestShortestCallChain(t *testing.T) {
	graph := BuildCallGraph([]parser.Function{
		{ID: "a", Name: "a", Callees: []parser.Callee{{Name: "c"}, {Name: "b"}}},
		{ID: "b", Name: "b", Callees: []parser.Callee{{Name: "d"}}},
		{ID: "c", Name: "c", Callees: []parser.Callee{{Name: "d"}}},
		{ID: "d", Name: "d", Callees: []parser.Callee{{Name: "a"}}},
	})
	for i := 0; i < 10; i++ {
		if got := graph.ShortestCallChain("a", "d", 2); !reflect.DeepEqual(got, []string{"a", "b", "d"}) {
			t.Fatal(got)
		}
	}
	if got := graph.ShortestCallChain("a", "d", 1); got != nil {
		t.Fatal(got)
	}
}
