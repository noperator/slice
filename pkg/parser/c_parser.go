package parser

import (
	"fmt"
	"os"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
)

func parseCFile(filename string) ([]Symbol, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	parser := sitter.NewParser()
	language := sitter.NewLanguage(tree_sitter_c.Language())
	err = parser.SetLanguage(language)
	if err != nil {
		return nil, err
	}

	tree := parser.Parse(content, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse file: %s", filename)
	}

	root := tree.RootNode()
	var symbols []Symbol

	symbols = append(symbols, findFunctionDefinitions(root, content, filename)...)

	return symbols, nil
}

func findFunctionDefinitions(node *sitter.Node, content []byte, filename string) []Symbol {
	var symbols []Symbol

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "function_definition" {
			symbol := analyzeFunctionDefinition(child, content, filename)
			if symbol != nil {
				symbols = append(symbols, *symbol)
			}
		}
		symbols = append(symbols, findFunctionDefinitions(child, content, filename)...)
	}

	return symbols
}

func analyzeFunctionDefinition(node *sitter.Node, content []byte, filename string) *Symbol {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	defText := getNodeText(node, content)
	symbol := &Symbol{
		Lang:      LangC,
		Kind:      "function",
		Filename:  filename,
		StartLine: int(startPoint.Row) + 1,
		EndLine:   int(endPoint.Row) + 1,
		Definition:                defText,
		DefinitionWithLineNumbers: addLineNumbers(defText, int(startPoint.Row)+1),
		Length:                    len(defText),
		Params:                    []Param{},
		Callees:                   []Callee{},
		Vars:                      []Variable{},
	}

	// Extract function signature and parameters
	declarator := findChildByType(node, "function_declarator")
	if declarator == nil {
		return nil
	}

	// Get function name
	identifier := findChildByType(declarator, "identifier")
	if identifier != nil {
		functionName := getNodeText(identifier, content)
		symbol.Name = functionName
		symbol.QualifiedName = functionName // C doesn't have qualified names

		// Generate function ID with language prefix
		symbol.ID = GenerateSymbolID(LangC, filename, symbol.StartLine, functionName)

		// Build signature - get return type
		returnType := ""
		for i := uint(0); i < node.ChildCount(); i++ {
			child := node.Child(i)
			if child.Kind() != "function_declarator" && child.Kind() != "compound_statement" {
				returnType += getNodeText(child, content) + " "
			} else if child.Kind() == "function_declarator" {
				break
			}
		}

		// Get parameters
		paramList := findChildByType(declarator, "parameter_list")
		if paramList != nil {
			symbol.Params = extractParameters(paramList, content)
		}

		// Build full signature
		var paramStrings []string
		for _, param := range symbol.Params {
			paramStrings = append(paramStrings, param.Snippet)
		}
		symbol.Signature = strings.TrimSpace(returnType) + " " + functionName + "(" + strings.Join(paramStrings, ", ") + ")"
	}

	// Find function body
	body := findChildByType(node, "compound_statement")
	if body != nil {
		// Extract function calls
		symbol.Callees = findFunctionCalls(body, content)

		// Extract variables
		symbol.Vars = findVariables(body, content, symbol.Params)
	}

	return symbol
}

func findChildByType(node *sitter.Node, nodeType string) *sitter.Node {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == nodeType {
			return child
		}
	}
	return nil
}

func extractParameters(paramList *sitter.Node, content []byte) []Param {
	var params []Param

	for i := uint(0); i < paramList.ChildCount(); i++ {
		child := paramList.Child(i)
		if child.Kind() == "parameter_declaration" {
			paramText := getNodeText(child, content)
			paramText = strings.TrimSpace(paramText)

			// Parse the parameter into components
			param := parseParameterDeclaration(paramText)
			if param != nil {
				params = append(params, *param)
			}
		}
	}

	return params
}

func parseParameterDeclaration(paramText string) *Param {
	if paramText == "" {
		return nil
	}

	// Split the parameter text into words
	words := strings.Fields(paramText)
	if len(words) == 0 {
		return nil
	}

	// The last word (possibly with * prefix) is the variable name
	lastWord := words[len(words)-1]

	// Extract the variable name by removing pointer indicators
	varName := strings.TrimLeft(lastWord, "*&")

	// The type is everything except the variable name
	var typeWords []string
	if len(words) > 1 {
		typeWords = words[:len(words)-1]

		// If the last word had pointer indicators, add them to the type
		if strings.HasPrefix(lastWord, "*") || strings.HasPrefix(lastWord, "&") {
			starCount := 0
			ampCount := 0
			for _, char := range lastWord {
				if char == '*' {
					starCount++
				} else if char == '&' {
					ampCount++
				} else {
					break
				}
			}

			if starCount > 0 {
				typeWords = append(typeWords, strings.Repeat("*", starCount))
			}
			if ampCount > 0 {
				typeWords = append(typeWords, strings.Repeat("&", ampCount))
			}
		}
	} else {
		// Single word parameter - treat as just the type
		typeWords = []string{lastWord}
		varName = ""
	}

	paramType := strings.Join(typeWords, " ")

	return &Param{
		Snippet: paramText,
		Name:    varName,
		Type:    strings.TrimSpace(paramType),
	}
}

func findFunctionCalls(node *sitter.Node, content []byte) []Callee {
	var callees []Callee

	// Recursively search for function calls
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "call_expression" {
			callee := analyzeFunctionCall(child, content)
			if callee != nil {
				callees = append(callees, *callee)
			}
		}
		// Recurse into child nodes
		callees = append(callees, findFunctionCalls(child, content)...)
	}

	return callees
}

func analyzeFunctionCall(node *sitter.Node, content []byte) *Callee {
	// Get function name
	functionNode := node.Child(0)
	if functionNode == nil {
		return nil
	}

	functionName := getNodeText(functionNode, content)
	lineNum := int(node.StartPosition().Row) + 1

	// Get arguments
	var args []string
	argList := findChildByType(node, "argument_list")
	if argList != nil {
		for i := uint(0); i < argList.ChildCount(); i++ {
			child := argList.Child(i)
			if child.Kind() != "," && child.Kind() != "(" && child.Kind() != ")" {
				argText := getNodeText(child, content)
				args = append(args, strings.TrimSpace(argText))
			}
		}
	}

	// Try to get the full statement by looking at parent context
	// Walk up the tree to find the statement containing this call
	snippet := getNodeText(node, content) // Default to just the call expression

	// Try to find the parent statement node
	parent := node.Parent()
	for parent != nil {
		parentKind := parent.Kind()
		if parentKind == "expression_statement" ||
			parentKind == "assignment_expression" ||
			parentKind == "declaration" ||
			parentKind == "init_declarator" ||
			parentKind == "return_statement" ||
			parentKind == "if_statement" ||
			parentKind == "while_statement" ||
			parentKind == "for_statement" {
			// Found a statement context - use its text
			snippet = strings.TrimSpace(getNodeText(parent, content))
			break
		}
		parent = parent.Parent()
	}

	return &Callee{
		Name:    functionName,
		Args:    args,
		Line:    lineNum,
		Snippet: snippet,
	}
}

func findVariables(node *sitter.Node, content []byte, params []Param) []Variable {
	varMap := make(map[string]*Variable)

	// Add function parameters as variables
	for _, param := range params {
		if param.Name != "" {
			varMap[param.Name] = &Variable{
				Name:   param.Name,
				Origin: "param",
				Type:   param.Type,
			}
		}
	}

	// Find basic local variable declarations
	findLocalVariableDeclarations(node, content, varMap)

	// Convert map to slice
	var variables []Variable
	for _, v := range varMap {
		variables = append(variables, *v)
	}

	return variables
}

// findLocalVariableDeclarations finds basic local variable declarations
func findLocalVariableDeclarations(node *sitter.Node, content []byte, varMap map[string]*Variable) {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)

		if child.Kind() == "declaration" {
			// Extract basic variable declarations without complex analysis
			extractBasicVariableDeclaration(child, content, varMap)
		}

		// Recurse into child nodes
		findLocalVariableDeclarations(child, content, varMap)
	}
}

// extractBasicVariableDeclaration extracts simple variable declarations
func extractBasicVariableDeclaration(node *sitter.Node, content []byte, varMap map[string]*Variable) {
	var typeParts []string
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "primitive_type" || child.Kind() == "type_identifier" ||
			child.Kind() == "struct_specifier" || child.Kind() == "storage_class_specifier" {
			typeParts = append(typeParts, getNodeText(child, content))
		}
	}

	declarationType := "unknown"
	if len(typeParts) > 0 {
		declarationType = strings.Join(typeParts, " ")
	}

	// Find declarators
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "init_declarator" || child.Kind() == "identifier" {
			varName := ""
			if child.Kind() == "identifier" {
				varName = getNodeText(child, content)
			} else {
				// init_declarator - find the identifier
				identifier := findChildByType(child, "identifier")
				if identifier != nil {
					varName = getNodeText(identifier, content)
				}
			}

			if varName != "" && varMap[varName] == nil {
				varMap[varName] = &Variable{
					Name:   varName,
					Origin: "local",
					Type:   declarationType,
				}
			}
		}
	}
}
