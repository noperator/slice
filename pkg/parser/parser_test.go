package parser

import (
	"path/filepath"
	"testing"
)

func TestParseCFile(t *testing.T) {
	testFile := filepath.Join("testdata", "c", "functions.c")

	symbols, err := parseCFile(testFile)
	if err != nil {
		t.Fatalf("Failed to parse C file: %v", err)
	}

	// Check we found the expected number of functions
	expectedCount := 5 // simple_function, add, process_data, compute, main
	if len(symbols) != expectedCount {
		t.Errorf("Expected %d symbols, got %d", expectedCount, len(symbols))
	}

	// Check language is set correctly
	for _, sym := range symbols {
		if sym.Lang != LangC {
			t.Errorf("Expected language %s, got %s for symbol %s", LangC, sym.Lang, sym.Name)
		}
		if sym.Kind != "function" {
			t.Errorf("Expected kind 'function', got %s for symbol %s", sym.Kind, sym.Name)
		}
	}

	// Test specific function: add
	var addFunc *Symbol
	for i := range symbols {
		if symbols[i].Name == "add" {
			addFunc = &symbols[i]
			break
		}
	}

	if addFunc == nil {
		t.Fatal("Expected to find 'add' function")
	}

	// Check add function details
	if addFunc.QualifiedName != "add" {
		t.Errorf("Expected qualified name 'add', got %s", addFunc.QualifiedName)
	}

	if len(addFunc.Params) != 2 {
		t.Errorf("Expected 2 parameters for add, got %d", len(addFunc.Params))
	}

	// Check ID format
	expectedIDPrefix := "c:" + testFile
	if len(addFunc.ID) < len(expectedIDPrefix) || addFunc.ID[:len(expectedIDPrefix)] != expectedIDPrefix {
		t.Errorf("Expected ID to start with %s, got %s", expectedIDPrefix, addFunc.ID)
	}

	// Test function with calls: compute
	var computeFunc *Symbol
	for i := range symbols {
		if symbols[i].Name == "compute" {
			computeFunc = &symbols[i]
			break
		}
	}

	if computeFunc == nil {
		t.Fatal("Expected to find 'compute' function")
	}

	// Check that compute has callees
	if len(computeFunc.Callees) == 0 {
		t.Error("Expected compute to have callees")
	}

	// Check for specific callee
	foundAdd := false
	foundSimple := false
	for _, callee := range computeFunc.Callees {
		if callee.Name == "add" {
			foundAdd = true
		}
		if callee.Name == "simple_function" {
			foundSimple = true
		}
	}

	if !foundAdd {
		t.Error("Expected compute to call 'add'")
	}
	if !foundSimple {
		t.Error("Expected compute to call 'simple_function'")
	}
}

func TestParseGoFile(t *testing.T) {
	testFile := filepath.Join("testdata", "go", "functions.go")

	symbols, err := parseGoFile(testFile)
	if err != nil {
		t.Fatalf("Failed to parse Go file: %v", err)
	}

	// Check we found symbols
	if len(symbols) == 0 {
		t.Fatal("Expected to find symbols in Go file")
	}

	// Count functions and methods
	var functionCount, methodCount int
	for _, sym := range symbols {
		if sym.Lang != LangGo {
			t.Errorf("Expected language %s, got %s for symbol %s", LangGo, sym.Lang, sym.Name)
		}
		switch sym.Kind {
		case "function":
			functionCount++
		case "method":
			methodCount++
		default:
			t.Errorf("Unexpected kind %s for symbol %s", sym.Kind, sym.Name)
		}
	}

	// We should have both functions and methods
	if functionCount == 0 {
		t.Error("Expected to find at least one function")
	}
	if methodCount == 0 {
		t.Error("Expected to find at least one method")
	}

	// Test specific function: Add
	var addFunc *Symbol
	for i := range symbols {
		if symbols[i].Name == "Add" && symbols[i].Kind == "function" {
			addFunc = &symbols[i]
			break
		}
	}

	if addFunc == nil {
		t.Fatal("Expected to find 'Add' function")
	}

	// Check qualified name includes package
	if addFunc.QualifiedName != "testdata.Add" {
		t.Errorf("Expected qualified name 'testdata.Add', got %s", addFunc.QualifiedName)
	}

	// Check parameters
	if len(addFunc.Params) != 2 {
		t.Errorf("Expected 2 parameters for Add, got %d", len(addFunc.Params))
	}

	// Check ID format
	expectedIDPrefix := "go:" + testFile
	if len(addFunc.ID) < len(expectedIDPrefix) || addFunc.ID[:len(expectedIDPrefix)] != expectedIDPrefix {
		t.Errorf("Expected ID to start with %s, got %s", expectedIDPrefix, addFunc.ID)
	}

	// Test method: Calculator.Add
	var calcAddMethod *Symbol
	for i := range symbols {
		if symbols[i].Name == "Add" && symbols[i].Kind == "method" {
			calcAddMethod = &symbols[i]
			break
		}
	}

	if calcAddMethod == nil {
		t.Fatal("Expected to find Calculator.Add method")
	}

	// Check qualified name for method
	if calcAddMethod.QualifiedName != "Calculator.Add" {
		t.Errorf("Expected qualified name 'Calculator.Add', got %s", calcAddMethod.QualifiedName)
	}

	// Check metadata for receiver
	if calcAddMethod.Metadata == nil {
		t.Error("Expected metadata for method")
	} else {
		if _, ok := calcAddMethod.Metadata["receiver"]; !ok {
			t.Error("Expected receiver in metadata")
		}
		if _, ok := calcAddMethod.Metadata["receiver_type"]; !ok {
			t.Error("Expected receiver_type in metadata")
		}
	}

	// Test function with calls: Compute
	var computeFunc *Symbol
	for i := range symbols {
		if symbols[i].Name == "Compute" {
			computeFunc = &symbols[i]
			break
		}
	}

	if computeFunc == nil {
		t.Fatal("Expected to find 'Compute' function")
	}

	// Check that Compute has callees
	if len(computeFunc.Callees) == 0 {
		t.Error("Expected Compute to have callees")
	}

	// Check for specific callees
	foundAdd := false
	foundNewCalculator := false
	for _, callee := range computeFunc.Callees {
		if callee.Name == "Add" {
			foundAdd = true
		}
		if callee.Name == "NewCalculator" {
			foundNewCalculator = true
		}
	}

	if !foundAdd {
		t.Error("Expected Compute to call 'Add'")
	}
	if !foundNewCalculator {
		t.Error("Expected Compute to call 'NewCalculator'")
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		filepath string
		expected string
		wantErr  bool
	}{
		{"test.go", LangGo, false},
		{"test.c", LangC, false},
		{"test.h", LangC, false},
		{"test.GO", LangGo, false}, // Case insensitive
		{"test.C", LangC, false},
		{"test.txt", "", true}, // Unsupported
		{"test.py", "", true},  // Not yet supported
	}

	for _, tt := range tests {
		lang, err := DetectLanguage(tt.filepath)
		if tt.wantErr {
			if err == nil {
				t.Errorf("DetectLanguage(%s) expected error, got nil", tt.filepath)
			}
		} else {
			if err != nil {
				t.Errorf("DetectLanguage(%s) unexpected error: %v", tt.filepath, err)
			}
			if lang != tt.expected {
				t.Errorf("DetectLanguage(%s) = %s, want %s", tt.filepath, lang, tt.expected)
			}
		}
	}
}

func TestParseFile(t *testing.T) {
	// Test C file
	cFile := filepath.Join("testdata", "c", "functions.c")
	cSymbols, err := ParseFile(cFile)
	if err != nil {
		t.Fatalf("ParseFile(%s) failed: %v", cFile, err)
	}
	if len(cSymbols) == 0 {
		t.Errorf("ParseFile(%s) returned no symbols", cFile)
	}
	for _, sym := range cSymbols {
		if sym.Lang != LangC {
			t.Errorf("Expected C symbols, got %s", sym.Lang)
		}
	}

	// Test Go file
	goFile := filepath.Join("testdata", "go", "functions.go")
	goSymbols, err := ParseFile(goFile)
	if err != nil {
		t.Fatalf("ParseFile(%s) failed: %v", goFile, err)
	}
	if len(goSymbols) == 0 {
		t.Errorf("ParseFile(%s) returned no symbols", goFile)
	}
	for _, sym := range goSymbols {
		if sym.Lang != LangGo {
			t.Errorf("Expected Go symbols, got %s", sym.Lang)
		}
	}
}

func TestAnalyzeDirectory(t *testing.T) {
	testDir := "testdata"

	result, err := analyzeDirectory(testDir)
	if err != nil {
		t.Fatalf("analyzeDirectory failed: %v", err)
	}

	// Check we found symbols
	if len(result.Symbols) == 0 {
		t.Fatal("Expected to find symbols")
	}

	// Check we have both languages represented
	foundC := false
	foundGo := false
	for _, sym := range result.Symbols {
		if sym.Lang == LangC {
			foundC = true
		}
		if sym.Lang == LangGo {
			foundGo = true
		}
	}

	if !foundC {
		t.Error("Expected to find at least one C symbol")
	}
	if !foundGo {
		t.Error("Expected to find at least one Go symbol")
	}

	// Verify no ID collisions
	idSet := make(map[string]bool)
	for _, sym := range result.Symbols {
		if idSet[sym.ID] {
			t.Errorf("Duplicate ID found: %s", sym.ID)
		}
		idSet[sym.ID] = true
	}
}

func TestGenerateSymbolID(t *testing.T) {
	tests := []struct {
		lang     string
		file     string
		line     int
		qualified string
		want     string
	}{
		{LangC, "test.c", 10, "main", "c:test.c:10:main"},
		{LangGo, "test.go", 20, "pkg.Function", "go:test.go:20:pkg.Function"},
		{LangGo, "helper.go", 15, "Type.Method", "go:helper.go:15:Type.Method"},
	}

	for _, tt := range tests {
		got := GenerateSymbolID(tt.lang, tt.file, tt.line, tt.qualified)
		if got != tt.want {
			t.Errorf("GenerateSymbolID(%s, %s, %d, %s) = %s, want %s",
				tt.lang, tt.file, tt.line, tt.qualified, got, tt.want)
		}
	}
}
