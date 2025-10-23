package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func analyzeDirectory(dir string) (*AnalysisResult, error) {
	result := &AnalysisResult{
		Symbols: []Symbol{},
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Detect language by extension
		_, err = DetectLanguage(path)
		if err != nil {
			// Skip unsupported files silently
			return nil
		}

		// Parse the file
		symbols, err := ParseFile(path)
		if err != nil {
			// Skip files that fail to parse
			return nil
		}

		result.Symbols = append(result.Symbols, symbols...)

		return nil
	})

	return result, err
}

// GenerateSymbolID creates a unique identifier for a symbol across languages
// Format: <lang>:<file>:<line>:<qualified_name>
// Examples:
//   - "go:pkg/parser/parser.go:42:parser.Parse"
//   - "c:src/main.c:100:main"
func GenerateSymbolID(lang, file string, line int, qualifiedName string) string {
	return fmt.Sprintf("%s:%s:%d:%s", lang, file, line, qualifiedName)
}

// getNodeText extracts the text content from a tree-sitter node
func getNodeText(node *sitter.Node, content []byte) string {
	startByte := node.StartByte()
	endByte := node.EndByte()
	return string(content[startByte:endByte])
}

// addLineNumbers adds right-aligned, zero-padded line numbers to each line of text
// Format: "NNNNN  CCC..." where N is the line number (5 digits, space-padded), followed by two spaces, followed by code
func addLineNumbers(text string, startLine int) string {
	lines := strings.Split(text, "\n")
	var result strings.Builder

	for i, line := range lines {
		lineNum := startLine + i
		// Format line number as right-aligned, space-padded 5-digit number
		result.WriteString(fmt.Sprintf("%5d  %s", lineNum, line))

		// Add newline except for the last line (to preserve original text structure)
		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// Simple cache for parsed analysis results
var (
	cache      = make(map[string]*AnalysisResult)
	cacheMutex sync.RWMutex
)

// GetCachedAnalysisResult returns cached analysis result for a directory, parsing if needed
func GetCachedAnalysisResult(directory string) (*AnalysisResult, error) {
	cacheMutex.RLock()
	if result, exists := cache[directory]; exists {
		cacheMutex.RUnlock()
		return result, nil
	}
	cacheMutex.RUnlock()

	// Parse the directory
	result, err := analyzeDirectory(directory)
	if err != nil {
		return nil, err
	}

	// Cache the results
	cacheMutex.Lock()
	cache[directory] = result
	cacheMutex.Unlock()

	return result, nil
}

// FindFunctionByID finds a symbol (function) by its ID in cached results
func FindFunctionByID(directory, functionID string) (*Symbol, error) {
	result, err := GetCachedAnalysisResult(directory)
	if err != nil {
		return nil, err
	}

	for i := range result.Symbols {
		if result.Symbols[i].ID == functionID {
			return &result.Symbols[i], nil
		}
	}

	return nil, fmt.Errorf("function not found: %s", functionID)
}
