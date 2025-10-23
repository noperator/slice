package parser

// Language constants
const (
	LangGo  = "go"
	LangC   = "c"
	LangCpp = "cpp" // Future
	LangPy  = "py"  // Future
	LangTS  = "ts"  // Future
	LangJS  = "js"  // Future
)

// Variable represents a variable declaration in source code
type Variable struct {
	Name   string `json:"name"`
	Origin string `json:"origin"`
	Type   string `json:"type"`
}

// Callee represents a function call site
type Callee struct {
	Name    string   `json:"name"`
	Args    []string `json:"args,omitempty"` // Keep for compatibility
	Line    int      `json:"line"`
	Snippet string   `json:"snippet"`
}

// Param represents a function parameter
type Param struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Snippet string `json:"snippet,omitempty"` // Keep for compatibility
}

// Symbol represents a code symbol (function, method, class, etc.) across multiple languages
type Symbol struct {
	// Core identification (unique across languages in monorepo)
	ID   string `json:"id"`   // Format: <lang>:<file>:<line>:<qualified_name>
	Lang string `json:"lang"` // "go", "c", etc.
	Kind string `json:"kind"` // "function", "method", "class", "interface", "struct"

	// Location
	Filename  string `json:"file"`
	StartLine int    `json:"start"`
	EndLine   int    `json:"end"`

	// Identity
	Name          string `json:"name"`           // Simple name: "Parse"
	QualifiedName string `json:"qualified_name"` // Go: "pkg.Type.Method", C: "parse_data"

	// Source code
	Definition                string `json:"def"`
	DefinitionWithLineNumbers string `json:"def_ln"`
	Signature                 string `json:"sig"`
	Length                    int    `json:"len"`

	// Relationships (language-agnostic)
	Callees []Callee `json:"callees"`
	Params  []Param  `json:"params"`

	// Language-specific metadata (use sparingly)
	Metadata map[string]interface{} `json:"meta,omitempty"`

	// Legacy fields for backward compatibility with C parser
	Vars []Variable `json:"vars,omitempty"`
}

// AnalysisResult contains the results of analyzing a codebase
type AnalysisResult struct {
	Symbols []Symbol `json:"symbols"`
}

// Function is an alias for Symbol for backward compatibility
// This allows existing code to continue working while we migrate
type Function = Symbol

// Parameter is an alias for Param for backward compatibility
type Parameter = Param
