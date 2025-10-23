package analysis

import (
	"testing"

	"github.com/noperator/slice/pkg/parser"
)

func TestLanguageAwareCallResolution(t *testing.T) {
	// Create symbols with the same function name in different languages
	symbols := []parser.Symbol{
		{
			ID:            "c:main.c:10:helper",
			Lang:          parser.LangC,
			Kind:          "function",
			Name:          "helper",
			QualifiedName: "helper",
			Callees: []parser.Callee{
				{Name: "process", Line: 11},
			},
		},
		{
			ID:            "c:main.c:20:process",
			Lang:          parser.LangC,
			Kind:          "function",
			Name:          "process",
			QualifiedName: "process",
			Callees:       []parser.Callee{},
		},
		{
			ID:            "go:main.go:10:main.Helper",
			Lang:          parser.LangGo,
			Kind:          "function",
			Name:          "Helper",
			QualifiedName: "main.Helper",
			Callees: []parser.Callee{
				{Name: "Process", Line: 11},
			},
		},
		{
			ID:            "go:main.go:20:main.Process",
			Lang:          parser.LangGo,
			Kind:          "function",
			Name:          "Process",
			QualifiedName: "main.Process",
			Callees:       []parser.Callee{},
		},
	}

	cg := BuildCallGraph(symbols)

	// Test byLang index
	if len(cg.byLang[parser.LangC]) != 2 {
		t.Errorf("Expected 2 C symbols, got %d", len(cg.byLang[parser.LangC]))
	}
	if len(cg.byLang[parser.LangGo]) != 2 {
		t.Errorf("Expected 2 Go symbols, got %d", len(cg.byLang[parser.LangGo]))
	}

	// Test findInLanguage
	processInC := cg.findInLanguage("process", parser.LangC)
	if len(processInC) != 1 {
		t.Errorf("Expected 1 C 'process' function, got %d", len(processInC))
	}
	if len(processInC) > 0 && processInC[0] != "c:main.c:20:process" {
		t.Errorf("Expected C process ID, got %s", processInC[0])
	}

	processInGo := cg.findInLanguage("Process", parser.LangGo)
	if len(processInGo) != 1 {
		t.Errorf("Expected 1 Go 'Process' function, got %d", len(processInGo))
	}
	if len(processInGo) > 0 && processInGo[0] != "go:main.go:20:main.Process" {
		t.Errorf("Expected Go Process ID, got %s", processInGo[0])
	}

	// Test that calls are resolved within the same language
	// C helper should only call C process, not Go Process
	adjMap, _ := cg.g.AdjacencyMap()
	cHelperEdges := adjMap["c:main.c:10:helper"]
	if len(cHelperEdges) != 1 {
		t.Errorf("Expected C helper to have 1 edge, got %d", len(cHelperEdges))
	}
	if _, ok := cHelperEdges["c:main.c:20:process"]; !ok {
		t.Error("Expected C helper to call C process")
	}

	// Go Helper should only call Go Process, not C process
	goHelperEdges := adjMap["go:main.go:10:main.Helper"]
	if len(goHelperEdges) != 1 {
		t.Errorf("Expected Go Helper to have 1 edge, got %d", len(goHelperEdges))
	}
	if _, ok := goHelperEdges["go:main.go:20:main.Process"]; !ok {
		t.Error("Expected Go Helper to call Go Process")
	}
}

func TestLanguageScopedCallsOnly(t *testing.T) {
	// Test that calls are only resolved within the same language
	// No cross-language edges should be created
	symbols := []parser.Symbol{
		{
			ID:            "c:main.c:10:main",
			Lang:          parser.LangC,
			Kind:          "function",
			Name:          "main",
			QualifiedName: "main",
			Callees: []parser.Callee{
				{Name: "helper", Line: 11}, // exists in C
				{Name: "strlen", Line: 12}, // doesn't exist (stdlib)
			},
		},
		{
			ID:            "c:util.c:5:helper",
			Lang:          parser.LangC,
			Kind:          "function",
			Name:          "helper",
			QualifiedName: "helper",
			Callees:       []parser.Callee{},
		},
	}

	cg := BuildCallGraph(symbols)

	// main should have edge to helper (found in same language)
	adjMap, _ := cg.g.AdjacencyMap()
	mainEdges := adjMap["c:main.c:10:main"]
	if len(mainEdges) != 1 {
		t.Errorf("Expected main to have 1 edge, got %d", len(mainEdges))
	}

	if _, ok := mainEdges["c:util.c:5:helper"]; !ok {
		t.Error("Expected main to call helper")
	}

	// strlen won't be found (not in our symbols), so no edge created
	// This is expected behavior - we only track calls to functions we've parsed
	// and only within the same language
}

func TestNameCollisionPrevention(t *testing.T) {
	// Test that functions with the same simple name but in different
	// languages are properly distinguished
	symbols := []parser.Symbol{
		{
			ID:            "c:main.c:5:main",
			Lang:          parser.LangC,
			Kind:          "function",
			Name:          "main",
			QualifiedName: "main",
			Callees: []parser.Callee{
				{Name: "init", Line: 6}, // C init
			},
		},
		{
			ID:            "c:main.c:10:init",
			Lang:          parser.LangC,
			Kind:          "function",
			Name:          "init",
			QualifiedName: "init",
			Callees:       []parser.Callee{},
		},
		{
			ID:            "go:main.go:5:main.main",
			Lang:          parser.LangGo,
			Kind:          "function",
			Name:          "main",
			QualifiedName: "main.main",
			Callees: []parser.Callee{
				{Name: "init", Line: 6}, // Go init
			},
		},
		{
			ID:            "go:main.go:10:main.init",
			Lang:          parser.LangGo,
			Kind:          "function",
			Name:          "init",
			QualifiedName: "main.init",
			Callees:       []parser.Callee{},
		},
	}

	cg := BuildCallGraph(symbols)

	// Both main functions should exist as separate vertices
	if len(cg.functions["main"]) != 2 {
		t.Errorf("Expected 2 'main' functions, got %d", len(cg.functions["main"]))
	}

	// Both init functions should exist as separate vertices
	if len(cg.functions["init"]) != 2 {
		t.Errorf("Expected 2 'init' functions, got %d", len(cg.functions["init"]))
	}

	// C main should only call C init
	adjMap, _ := cg.g.AdjacencyMap()
	cMainEdges := adjMap["c:main.c:5:main"]
	if len(cMainEdges) != 1 {
		t.Errorf("Expected C main to have 1 edge, got %d", len(cMainEdges))
	}
	if _, ok := cMainEdges["c:main.c:10:init"]; !ok {
		t.Error("Expected C main to call C init")
	}

	// Go main should only call Go init
	goMainEdges := adjMap["go:main.go:5:main.main"]
	if len(goMainEdges) != 1 {
		t.Errorf("Expected Go main to have 1 edge, got %d", len(goMainEdges))
	}
	if _, ok := goMainEdges["go:main.go:10:main.init"]; !ok {
		t.Error("Expected Go main to call Go init")
	}
}
