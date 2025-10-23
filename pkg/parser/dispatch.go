package parser

import (
	"fmt"
	"path"
	"strings"
)

// ExtensionToLang maps file extensions to language identifiers
var ExtensionToLang = map[string]string{
	".go": LangGo,
	".c":  LangC,
	".h":  LangC,
	// Future extensions commented out:
	// ".cpp": LangCpp,
	// ".cc":  LangCpp,
	// ".cxx": LangCpp,
	// ".hpp": LangCpp,
	// ".hxx": LangCpp,
	// ".py":  LangPy,
	// ".ts":  LangTS,
	// ".js":  LangJS,
	// ".jsx": LangJS,
	// ".tsx": LangTS,
}

// DetectLanguage determines the programming language of a file based on its extension
func DetectLanguage(filepath string) (string, error) {
	ext := strings.ToLower(path.Ext(filepath))
	if lang, ok := ExtensionToLang[ext]; ok {
		return lang, nil
	}
	return "", fmt.Errorf("unsupported file extension: %s", ext)
}

// ParseFile analyzes a source file and extracts symbols (functions, methods, etc.)
// It automatically detects the language and dispatches to the appropriate parser
func ParseFile(filepath string) ([]Symbol, error) {
	lang, err := DetectLanguage(filepath)
	if err != nil {
		return nil, err
	}

	switch lang {
	case LangGo:
		return parseGoFile(filepath)
	case LangC:
		return parseCFile(filepath)
	default:
		return nil, fmt.Errorf("parser not implemented for language: %s", lang)
	}
}
