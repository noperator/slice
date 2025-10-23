package parser

import (
	"fmt"
	"os"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func parseTypeScriptFile(filename string) ([]Symbol, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	parser := sitter.NewParser()
	language := sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
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

	// Find top-level functions and classes
	symbols = append(symbols, findTypeScriptFunctions(root, content, filename)...)
	symbols = append(symbols, findTypeScriptClasses(root, content, filename)...)

	return symbols, nil
}

func findTypeScriptFunctions(node *sitter.Node, content []byte, filename string) []Symbol {
	var symbols []Symbol

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "function_declaration" {
			symbol := analyzeTypeScriptFunction(child, content, filename)
			if symbol != nil {
				symbols = append(symbols, *symbol)
			}
		} else if child.Kind() == "lexical_declaration" {
			// Check for arrow functions: const foo = () => {}
			for j := uint(0); j < child.ChildCount(); j++ {
				varDecl := child.Child(j)
				if varDecl.Kind() == "variable_declarator" {
					// Check if the value is an arrow function
					for k := uint(0); k < varDecl.ChildCount(); k++ {
						valueNode := varDecl.Child(k)
						if valueNode.Kind() == "arrow_function" {
							symbol := analyzeTypeScriptArrowFunction(varDecl, content, filename)
							if symbol != nil {
								symbols = append(symbols, *symbol)
							}
							break
						}
					}
				}
			}
		}
		// Recurse but skip class bodies
		if child.Kind() != "class_declaration" {
			symbols = append(symbols, findTypeScriptFunctions(child, content, filename)...)
		}
	}

	return symbols
}

func findTypeScriptClasses(node *sitter.Node, content []byte, filename string) []Symbol {
	var symbols []Symbol

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "class_declaration" {
			// Add the class itself
			classSymbol := analyzeTypeScriptClass(child, content, filename)
			if classSymbol != nil {
				symbols = append(symbols, *classSymbol)

				// Find methods within this class
				className := classSymbol.Name
				methods := findTypeScriptMethods(child, content, filename, className)
				symbols = append(symbols, methods...)
			}
		}
		// Recurse to find nested classes
		symbols = append(symbols, findTypeScriptClasses(child, content, filename)...)
	}

	return symbols
}

func findTypeScriptMethods(classNode *sitter.Node, content []byte, filename string, className string) []Symbol {
	var symbols []Symbol

	// Look for class_body
	for i := uint(0); i < classNode.ChildCount(); i++ {
		child := classNode.Child(i)
		if child.Kind() == "class_body" {
			// Find method_definitions within the class body
			for j := uint(0); j < child.ChildCount(); j++ {
				methodNode := child.Child(j)
				if methodNode.Kind() == "method_definition" {
					method := analyzeTypeScriptMethod(methodNode, content, filename, className)
					if method != nil {
						symbols = append(symbols, *method)
					}
				}
			}
		}
	}

	return symbols
}

func analyzeTypeScriptFunction(node *sitter.Node, content []byte, filename string) *Symbol {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	defText := getNodeText(node, content)
	symbol := &Symbol{
		Lang:                      LangTS,
		Kind:                      "function",
		Filename:                  filename,
		StartLine:                 int(startPoint.Row) + 1,
		EndLine:                   int(endPoint.Row) + 1,
		Definition:                defText,
		DefinitionWithLineNumbers: addLineNumbers(defText, int(startPoint.Row)+1),
		Length:                    len(defText),
		Params:                    []Param{},
		Callees:                   []Callee{},
		Vars:                      []Variable{},
	}

	// Get function name
	nameNode := findChildByType(node, "identifier")
	if nameNode == nil {
		return nil
	}

	functionName := getNodeText(nameNode, content)
	symbol.Name = functionName
	symbol.QualifiedName = functionName

	// Generate function ID with language prefix
	symbol.ID = GenerateSymbolID(LangTS, filename, symbol.StartLine, functionName)

	// Get parameters
	paramNode := findChildByType(node, "formal_parameters")
	if paramNode != nil {
		symbol.Params = extractTypeScriptParameters(paramNode, content)
	}

	// Build signature
	var paramStrings []string
	for _, param := range symbol.Params {
		paramStrings = append(paramStrings, param.Snippet)
	}

	// Get return type
	returnType := ""
	typeAnnotation := findChildByType(node, "type_annotation")
	if typeAnnotation != nil {
		returnType = ": " + strings.TrimSpace(getNodeText(typeAnnotation, content))
	}

	symbol.Signature = "function " + functionName + "(" + strings.Join(paramStrings, ", ") + ")" + returnType

	// Find function body
	body := findChildByType(node, "statement_block")
	if body != nil {
		// Extract function calls
		symbol.Callees = findTypeScriptFunctionCalls(body, content)
	}

	return symbol
}

func analyzeTypeScriptArrowFunction(varDeclarator *sitter.Node, content []byte, filename string) *Symbol {
	startPoint := varDeclarator.StartPosition()
	endPoint := varDeclarator.EndPosition()

	defText := getNodeText(varDeclarator, content)
	symbol := &Symbol{
		Lang:                      LangTS,
		Kind:                      "function",
		Filename:                  filename,
		StartLine:                 int(startPoint.Row) + 1,
		EndLine:                   int(endPoint.Row) + 1,
		Definition:                defText,
		DefinitionWithLineNumbers: addLineNumbers(defText, int(startPoint.Row)+1),
		Length:                    len(defText),
		Params:                    []Param{},
		Callees:                   []Callee{},
		Vars:                      []Variable{},
	}

	// Get function name from variable declarator
	nameNode := findChildByType(varDeclarator, "identifier")
	if nameNode == nil {
		return nil
	}

	functionName := getNodeText(nameNode, content)
	symbol.Name = functionName
	symbol.QualifiedName = functionName

	// Generate function ID with language prefix
	symbol.ID = GenerateSymbolID(LangTS, filename, symbol.StartLine, functionName)

	// Get arrow function node
	arrowFunc := findChildByType(varDeclarator, "arrow_function")
	if arrowFunc != nil {
		// Get parameters
		paramNode := findChildByType(arrowFunc, "formal_parameters")
		if paramNode != nil {
			symbol.Params = extractTypeScriptParameters(paramNode, content)
		}

		// Build signature
		var paramStrings []string
		for _, param := range symbol.Params {
			paramStrings = append(paramStrings, param.Snippet)
		}

		// Get return type
		returnType := ""
		typeAnnotation := findChildByType(arrowFunc, "type_annotation")
		if typeAnnotation != nil {
			returnType = ": " + strings.TrimSpace(getNodeText(typeAnnotation, content))
		}

		symbol.Signature = "const " + functionName + " = (" + strings.Join(paramStrings, ", ") + ")" + returnType + " => {...}"

		// Find function body
		body := findChildByType(arrowFunc, "statement_block")
		if body != nil {
			// Extract function calls
			symbol.Callees = findTypeScriptFunctionCalls(body, content)
		}
	}

	return symbol
}

func analyzeTypeScriptMethod(node *sitter.Node, content []byte, filename string, className string) *Symbol {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	defText := getNodeText(node, content)
	symbol := &Symbol{
		Lang:                      LangTS,
		Kind:                      "method",
		Filename:                  filename,
		StartLine:                 int(startPoint.Row) + 1,
		EndLine:                   int(endPoint.Row) + 1,
		Definition:                defText,
		DefinitionWithLineNumbers: addLineNumbers(defText, int(startPoint.Row)+1),
		Length:                    len(defText),
		Params:                    []Param{},
		Callees:                   []Callee{},
		Vars:                      []Variable{},
		Metadata:                  make(map[string]interface{}),
	}

	// Store class name in metadata
	symbol.Metadata["class_name"] = className

	// Get method name (could be property_identifier or identifier)
	var nameNode *sitter.Node
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "property_identifier" || child.Kind() == "identifier" {
			nameNode = child
			break
		}
	}

	if nameNode == nil {
		return nil
	}

	methodName := getNodeText(nameNode, content)
	symbol.Name = methodName
	symbol.QualifiedName = className + "." + methodName

	// Generate method ID with language prefix
	symbol.ID = GenerateSymbolID(LangTS, filename, symbol.StartLine, symbol.QualifiedName)

	// Get parameters
	paramNode := findChildByType(node, "formal_parameters")
	if paramNode != nil {
		symbol.Params = extractTypeScriptParameters(paramNode, content)
	}

	// Build signature
	var paramStrings []string
	for _, param := range symbol.Params {
		paramStrings = append(paramStrings, param.Snippet)
	}

	// Get return type
	returnType := ""
	typeAnnotation := findChildByType(node, "type_annotation")
	if typeAnnotation != nil {
		returnType = ": " + strings.TrimSpace(getNodeText(typeAnnotation, content))
	}

	symbol.Signature = methodName + "(" + strings.Join(paramStrings, ", ") + ")" + returnType

	// Find method body
	body := findChildByType(node, "statement_block")
	if body != nil {
		// Extract function calls
		symbol.Callees = findTypeScriptFunctionCalls(body, content)
	}

	return symbol
}

func analyzeTypeScriptClass(node *sitter.Node, content []byte, filename string) *Symbol {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	defText := getNodeText(node, content)
	symbol := &Symbol{
		Lang:                      LangTS,
		Kind:                      "class",
		Filename:                  filename,
		StartLine:                 int(startPoint.Row) + 1,
		EndLine:                   int(endPoint.Row) + 1,
		Definition:                defText,
		DefinitionWithLineNumbers: addLineNumbers(defText, int(startPoint.Row)+1),
		Length:                    len(defText),
		Params:                    []Param{},
		Callees:                   []Callee{},
		Vars:                      []Variable{},
	}

	// Get class name (could be type_identifier or identifier)
	var nameNode *sitter.Node
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "type_identifier" || child.Kind() == "identifier" {
			nameNode = child
			break
		}
	}

	if nameNode == nil {
		return nil
	}

	className := getNodeText(nameNode, content)
	symbol.Name = className
	symbol.QualifiedName = className
	symbol.ID = GenerateSymbolID(LangTS, filename, symbol.StartLine, className)
	symbol.Signature = "class " + className

	return symbol
}

func extractTypeScriptParameters(paramNode *sitter.Node, content []byte) []Param {
	var params []Param

	for i := uint(0); i < paramNode.ChildCount(); i++ {
		child := paramNode.Child(i)
		// Skip parentheses and commas
		if child.Kind() == "required_parameter" || child.Kind() == "optional_parameter" {
			paramText := getNodeText(child, content)
			// Try to extract name
			identifier := findChildByType(child, "identifier")
			paramName := ""
			if identifier != nil {
				paramName = getNodeText(identifier, content)
			}

			// Try to extract type
			paramType := ""
			typeAnnotation := findChildByType(child, "type_annotation")
			if typeAnnotation != nil {
				paramType = strings.TrimSpace(getNodeText(typeAnnotation, content))
			}

			params = append(params, Param{
				Name:    paramName,
				Type:    paramType,
				Snippet: paramText,
			})
		}
	}

	return params
}

func findTypeScriptFunctionCalls(node *sitter.Node, content []byte) []Callee {
	var callees []Callee

	// Recursively search for function calls
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "call_expression" {
			callee := analyzeTypeScriptFunctionCall(child, content)
			if callee != nil {
				callees = append(callees, *callee)
			}
		}
		// Recurse into child nodes
		callees = append(callees, findTypeScriptFunctionCalls(child, content)...)
	}

	return callees
}

func analyzeTypeScriptFunctionCall(node *sitter.Node, content []byte) *Callee {
	// Get function name
	functionNode := node.Child(0)
	if functionNode == nil {
		return nil
	}

	functionName := getNodeText(functionNode, content)
	lineNum := int(node.StartPosition().Row) + 1

	// Get arguments
	var args []string
	argList := findChildByType(node, "arguments")
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
	snippet := getNodeText(node, content) // Default to just the call expression

	// Try to find the parent statement node
	parent := node.Parent()
	for parent != nil {
		parentKind := parent.Kind()
		if parentKind == "expression_statement" ||
			parentKind == "variable_declaration" ||
			parentKind == "lexical_declaration" ||
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
