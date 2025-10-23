package codeql

import "github.com/noperator/slice/pkg/analysis"

type CodeQLResult struct {
	ObjName               string `json:"object"`
	FreeFunctionName      string `json:"free_func"`
	FreeFunctionFile      string `json:"free_file"`
	FreeFunctionDefLine   int    `json:"free_func_def_ln"`
	FreeLine              int    `json:"free_ln"`
	UseFunctionName       string `json:"use_func"`
	UseFunctionFile       string `json:"use_file"`
	UseFunctionDefLine    int    `json:"use_func_def_ln"`
	UseLine               int    `json:"use_ln"`
}

type FunctionCode struct {
	DefinitionWithLineNumbers string `json:"def"`
	Snippet                  string `json:"snippet"`
}

type SourceCode struct {
	FreeFunction          FunctionCode   `json:"free_func"`
	UseFunction           FunctionCode   `json:"use_func"`
	IntermediateFunctions []FunctionCode `json:"inter_funcs"`
}

type Finding struct {
	CodeQLResult   CodeQLResult              `json:"codeql_result"`
	SourceCode     SourceCode                `json:"source_code"`
	CallValidation *analysis.CallValidation  `json:"call_validation,omitempty"`
}

