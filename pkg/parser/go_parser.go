package parser

import (
	"fmt"
	"os"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

// parseGoFile analyzes a Go source file and extracts symbols
func parseGoFile(filename string) ([]Symbol, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	parser := sitter.NewParser()
	language := sitter.NewLanguage(tree_sitter_go.Language())
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

	// Extract package name for qualified names
	packageName := extractPackageName(root, content)

	// Find function and method definitions
	symbols = append(symbols, findGoFunctions(root, content, filename, packageName)...)
	symbols = append(symbols, findGoMethods(root, content, filename, packageName)...)

	return symbols, nil
}

// extractPackageName extracts the package name from the Go source file
func extractPackageName(root *sitter.Node, content []byte) string {
	for i := uint(0); i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child.Kind() == "package_clause" {
			// Find the package identifier
			for j := uint(0); j < child.ChildCount(); j++ {
				pkgChild := child.Child(j)
				if pkgChild.Kind() == "package_identifier" {
					return getNodeText(pkgChild, content)
				}
			}
		}
	}
	return ""
}

// findGoFunctions finds all function definitions in a Go AST
func findGoFunctions(node *sitter.Node, content []byte, filename, packageName string) []Symbol {
	var symbols []Symbol

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "function_declaration" {
			symbol := analyzeGoFunction(child, content, filename, packageName)
			if symbol != nil {
				symbols = append(symbols, *symbol)
			}
		}
		// Recurse into child nodes
		symbols = append(symbols, findGoFunctions(child, content, filename, packageName)...)
	}

	return symbols
}

// findGoMethods finds all method definitions in a Go AST
func findGoMethods(node *sitter.Node, content []byte, filename, packageName string) []Symbol {
	var symbols []Symbol

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "method_declaration" {
			symbol := analyzeGoMethod(child, content, filename, packageName)
			if symbol != nil {
				symbols = append(symbols, *symbol)
			}
		}
		// Recurse into child nodes
		symbols = append(symbols, findGoMethods(child, content, filename, packageName)...)
	}

	return symbols
}

// analyzeGoFunction analyzes a Go function declaration
func analyzeGoFunction(node *sitter.Node, content []byte, filename, packageName string) *Symbol {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	defText := getNodeText(node, content)
	symbol := &Symbol{
		Lang:                      LangGo,
		Kind:                      "function",
		Filename:                  filename,
		StartLine:                 int(startPoint.Row) + 1,
		EndLine:                   int(endPoint.Row) + 1,
		Definition:                defText,
		DefinitionWithLineNumbers: addLineNumbers(defText, int(startPoint.Row)+1),
		Length:                    len(defText),
		Params:                    []Param{},
		Callees:                   []Callee{},
		Metadata:                  make(map[string]interface{}),
	}

	// Get function name
	var functionName string
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "identifier" {
			functionName = getNodeText(child, content)
			symbol.Name = functionName
			break
		}
	}

	// Build qualified name
	if packageName != "" {
		symbol.QualifiedName = packageName + "." + functionName
	} else {
		symbol.QualifiedName = functionName
	}

	// Generate symbol ID
	symbol.ID = GenerateSymbolID(LangGo, filename, symbol.StartLine, symbol.QualifiedName)

	// Extract parameters
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "parameter_list" {
			symbol.Params = extractGoParameters(child, content)
			break
		}
	}

	// Build signature
	symbol.Signature = buildGoFunctionSignature(node, content, functionName)

	// Extract function calls from the body
	body := findChildByType(node, "block")
	if body != nil {
		symbol.Callees = findGoFunctionCalls(body, content)
	}

	return symbol
}

// analyzeGoMethod analyzes a Go method declaration
func analyzeGoMethod(node *sitter.Node, content []byte, filename, packageName string) *Symbol {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	defText := getNodeText(node, content)
	symbol := &Symbol{
		Lang:                      LangGo,
		Kind:                      "method",
		Filename:                  filename,
		StartLine:                 int(startPoint.Row) + 1,
		EndLine:                   int(endPoint.Row) + 1,
		Definition:                defText,
		DefinitionWithLineNumbers: addLineNumbers(defText, int(startPoint.Row)+1),
		Length:                    len(defText),
		Params:                    []Param{},
		Callees:                   []Callee{},
		Metadata:                  make(map[string]interface{}),
	}

	// Get method name
	var methodName string
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "field_identifier" {
			methodName = getNodeText(child, content)
			symbol.Name = methodName
			break
		}
	}

	// Extract receiver information
	var receiverType string
	var receiverText string
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "parameter_list" {
			// First parameter_list is the receiver
			receiverText = getNodeText(child, content)
			receiverType = extractReceiverType(child, content)
			symbol.Metadata["receiver"] = receiverText
			symbol.Metadata["receiver_type"] = receiverType
			break
		}
	}

	// Build qualified name: Type.Method
	if receiverType != "" {
		symbol.QualifiedName = receiverType + "." + methodName
	} else {
		symbol.QualifiedName = methodName
	}

	// Generate symbol ID
	symbol.ID = GenerateSymbolID(LangGo, filename, symbol.StartLine, symbol.QualifiedName)

	// Extract method parameters (skip the receiver)
	paramListCount := 0
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "parameter_list" {
			paramListCount++
			if paramListCount == 2 { // Second parameter_list is the actual parameters
				symbol.Params = extractGoParameters(child, content)
				break
			}
		}
	}

	// Build signature with receiver
	symbol.Signature = buildGoMethodSignature(node, content, receiverText, methodName)

	// Extract function calls from the body
	body := findChildByType(node, "block")
	if body != nil {
		symbol.Callees = findGoFunctionCalls(body, content)
	}

	return symbol
}

// extractReceiverType extracts the type name from a receiver parameter list
func extractReceiverType(receiverNode *sitter.Node, content []byte) string {
	for i := uint(0); i < receiverNode.ChildCount(); i++ {
		child := receiverNode.Child(i)
		if child.Kind() == "parameter_declaration" {
			// Look for type_identifier or pointer_type
			for j := uint(0); j < child.ChildCount(); j++ {
				typeNode := child.Child(j)
				if typeNode.Kind() == "type_identifier" {
					return getNodeText(typeNode, content)
				} else if typeNode.Kind() == "pointer_type" {
					// For pointer receivers, extract the base type
					for k := uint(0); k < typeNode.ChildCount(); k++ {
						baseType := typeNode.Child(k)
						if baseType.Kind() == "type_identifier" {
							return getNodeText(baseType, content)
						}
					}
				}
			}
		}
	}
	return ""
}

// extractGoParameters extracts parameters from a Go parameter list
func extractGoParameters(paramList *sitter.Node, content []byte) []Param {
	var params []Param

	for i := uint(0); i < paramList.ChildCount(); i++ {
		child := paramList.Child(i)
		if child.Kind() == "parameter_declaration" {
			// Go parameters can have multiple names with one type: (a, b int)
			paramText := getNodeText(child, content)
			paramText = strings.TrimSpace(paramText)

			// Try to extract individual parameter info
			param := parseGoParameter(child, content)
			if param != nil {
				params = append(params, *param)
			}
		}
	}

	return params
}

// parseGoParameter parses a Go parameter declaration
func parseGoParameter(paramNode *sitter.Node, content []byte) *Param {
	fullText := getNodeText(paramNode, content)
	fullText = strings.TrimSpace(fullText)

	var names []string
	var typeName string

	// Extract names and type
	for i := uint(0); i < paramNode.ChildCount(); i++ {
		child := paramNode.Child(i)
		switch child.Kind() {
		case "identifier":
			names = append(names, getNodeText(child, content))
		case "type_identifier", "pointer_type", "slice_type", "array_type", "map_type", "interface_type", "struct_type", "function_type":
			typeName = getNodeText(child, content)
		}
	}

	// If we have names, create a param for the first name
	// (Go allows multiple params with same type like "a, b int")
	var name string
	if len(names) > 0 {
		name = names[0]
	}

	return &Param{
		Snippet: fullText,
		Name:    name,
		Type:    typeName,
	}
}

// buildGoFunctionSignature builds a function signature string
func buildGoFunctionSignature(node *sitter.Node, content []byte, functionName string) string {
	var parts []string
	parts = append(parts, "func")
	parts = append(parts, functionName)

	// Add parameters
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "parameter_list" {
			parts = append(parts, getNodeText(child, content))
			break
		}
	}

	// Add return type if present
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "parameter_list" {
			// Check if there's a return type after the parameter list
			if i+1 < node.ChildCount() {
				nextNode := node.Child(i + 1)
				// Return types can be a single type or a parameter_list for multiple returns
				if nextNode.Kind() != "block" {
					parts = append(parts, getNodeText(nextNode, content))
				}
			}
			break
		}
	}

	return strings.Join(parts, " ")
}

// buildGoMethodSignature builds a method signature string with receiver
func buildGoMethodSignature(node *sitter.Node, content []byte, receiver, methodName string) string {
	var parts []string
	parts = append(parts, "func")
	parts = append(parts, receiver)
	parts = append(parts, methodName)

	// Add parameters (skip first parameter_list which is the receiver)
	paramListCount := 0
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "parameter_list" {
			paramListCount++
			if paramListCount == 2 {
				parts = append(parts, getNodeText(child, content))

				// Add return type if present
				if i+1 < node.ChildCount() {
					nextNode := node.Child(i + 1)
					if nextNode.Kind() != "block" {
						parts = append(parts, getNodeText(nextNode, content))
					}
				}
				break
			}
		}
	}

	return strings.Join(parts, " ")
}

// findGoFunctionCalls finds function calls in a Go code block
func findGoFunctionCalls(node *sitter.Node, content []byte) []Callee {
	var callees []Callee

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "call_expression" {
			callee := analyzeGoFunctionCall(child, content)
			if callee != nil {
				callees = append(callees, *callee)
			}
		}
		// Recurse into child nodes
		callees = append(callees, findGoFunctionCalls(child, content)...)
	}

	return callees
}

// analyzeGoFunctionCall analyzes a Go function call expression
func analyzeGoFunctionCall(node *sitter.Node, content []byte) *Callee {
	// Get function name (can be identifier or selector_expression like pkg.Func)
	functionNode := node.Child(0)
	if functionNode == nil {
		return nil
	}

	functionName := getNodeText(functionNode, content)
	lineNum := int(node.StartPosition().Row) + 1

	// Extract just the function name for simple matching
	// e.g., "fmt.Println" -> "Println", "Parse" -> "Parse"
	simpleName := functionName
	if strings.Contains(functionName, ".") {
		parts := strings.Split(functionName, ".")
		simpleName = parts[len(parts)-1]
	}

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

	// Get the full statement for context
	snippet := getNodeText(node, content)

	// Try to get more context from parent
	parent := node.Parent()
	for parent != nil {
		parentKind := parent.Kind()
		if parentKind == "expression_statement" ||
			parentKind == "short_var_declaration" ||
			parentKind == "var_declaration" ||
			parentKind == "assignment_statement" ||
			parentKind == "return_statement" ||
			parentKind == "if_statement" ||
			parentKind == "for_statement" {
			snippet = strings.TrimSpace(getNodeText(parent, content))
			break
		}
		parent = parent.Parent()
	}

	return &Callee{
		Name:    simpleName, // Use simple name for matching
		Args:    args,
		Line:    lineNum,
		Snippet: snippet,
	}
}
