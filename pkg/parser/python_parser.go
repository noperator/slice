package parser

import (
	"fmt"
	"os"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

func parsePythonFile(filename string) ([]Symbol, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	parser := sitter.NewParser()
	language := sitter.NewLanguage(tree_sitter_python.Language())
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
	symbols = append(symbols, findPythonFunctions(root, content, filename)...)
	symbols = append(symbols, findPythonClasses(root, content, filename)...)

	return symbols, nil
}

func findPythonFunctions(node *sitter.Node, content []byte, filename string) []Symbol {
	var symbols []Symbol

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "function_definition" {
			symbol := analyzePythonFunction(child, content, filename)
			if symbol != nil {
				symbols = append(symbols, *symbol)
			}
		} else if child.Kind() == "decorated_definition" {
			// Handle decorated functions
			funcDef := findChildByType(child, "function_definition")
			if funcDef != nil {
				symbol := analyzePythonFunction(funcDef, content, filename)
				if symbol != nil {
					symbols = append(symbols, *symbol)
				}
			}
		}
		// Recurse but skip class bodies
		if child.Kind() != "class_definition" {
			symbols = append(symbols, findPythonFunctions(child, content, filename)...)
		}
	}

	return symbols
}

func findPythonClasses(node *sitter.Node, content []byte, filename string) []Symbol {
	var symbols []Symbol

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "class_definition" {
			// Add the class itself
			classSymbol := analyzePythonClass(child, content, filename)
			if classSymbol != nil {
				symbols = append(symbols, *classSymbol)

				// Find methods within this class
				className := classSymbol.Name
				methods := findPythonMethods(child, content, filename, className)
				symbols = append(symbols, methods...)
			}
		} else if child.Kind() == "decorated_definition" {
			// Handle decorated classes
			classDef := findChildByType(child, "class_definition")
			if classDef != nil {
				classSymbol := analyzePythonClass(classDef, content, filename)
				if classSymbol != nil {
					symbols = append(symbols, *classSymbol)

					className := classSymbol.Name
					methods := findPythonMethods(classDef, content, filename, className)
					symbols = append(symbols, methods...)
				}
			}
		}
		// Recurse to find nested classes
		symbols = append(symbols, findPythonClasses(child, content, filename)...)
	}

	return symbols
}

func findPythonMethods(classNode *sitter.Node, content []byte, filename string, className string) []Symbol {
	var symbols []Symbol

	// Look for block (class body)
	for i := uint(0); i < classNode.ChildCount(); i++ {
		child := classNode.Child(i)
		if child.Kind() == "block" {
			// Find function_definitions within the class body
			for j := uint(0); j < child.ChildCount(); j++ {
				methodNode := child.Child(j)
				if methodNode.Kind() == "function_definition" {
					method := analyzePythonMethod(methodNode, content, filename, className)
					if method != nil {
						symbols = append(symbols, *method)
					}
				} else if methodNode.Kind() == "decorated_definition" {
					// Handle decorated methods
					funcDef := findChildByType(methodNode, "function_definition")
					if funcDef != nil {
						method := analyzePythonMethod(funcDef, content, filename, className)
						if method != nil {
							symbols = append(symbols, *method)
						}
					}
				}
			}
		}
	}

	return symbols
}

func analyzePythonFunction(node *sitter.Node, content []byte, filename string) *Symbol {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	defText := getNodeText(node, content)
	symbol := &Symbol{
		Lang:                      LangPy,
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
	symbol.ID = GenerateSymbolID(LangPy, filename, symbol.StartLine, functionName)

	// Get parameters
	paramNode := findChildByType(node, "parameters")
	if paramNode != nil {
		symbol.Params = extractPythonParameters(paramNode, content)
	}

	// Build signature
	var paramStrings []string
	for _, param := range symbol.Params {
		paramStrings = append(paramStrings, param.Snippet)
	}
	symbol.Signature = "def " + functionName + "(" + strings.Join(paramStrings, ", ") + ")"

	// Find function body
	body := findChildByType(node, "block")
	if body != nil {
		// Extract function calls
		symbol.Callees = findPythonFunctionCalls(body, content)
	}

	return symbol
}

func analyzePythonMethod(node *sitter.Node, content []byte, filename string, className string) *Symbol {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	defText := getNodeText(node, content)
	symbol := &Symbol{
		Lang:                      LangPy,
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

	// Get method name
	nameNode := findChildByType(node, "identifier")
	if nameNode == nil {
		return nil
	}

	methodName := getNodeText(nameNode, content)
	symbol.Name = methodName
	symbol.QualifiedName = className + "." + methodName

	// Generate method ID with language prefix
	symbol.ID = GenerateSymbolID(LangPy, filename, symbol.StartLine, symbol.QualifiedName)

	// Get parameters
	paramNode := findChildByType(node, "parameters")
	if paramNode != nil {
		symbol.Params = extractPythonParameters(paramNode, content)
	}

	// Build signature
	var paramStrings []string
	for _, param := range symbol.Params {
		paramStrings = append(paramStrings, param.Snippet)
	}
	symbol.Signature = "def " + methodName + "(" + strings.Join(paramStrings, ", ") + ")"

	// Find method body
	body := findChildByType(node, "block")
	if body != nil {
		// Extract function calls
		symbol.Callees = findPythonFunctionCalls(body, content)
	}

	return symbol
}

func analyzePythonClass(node *sitter.Node, content []byte, filename string) *Symbol {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	defText := getNodeText(node, content)
	symbol := &Symbol{
		Lang:                      LangPy,
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

	// Get class name
	nameNode := findChildByType(node, "identifier")
	if nameNode == nil {
		return nil
	}

	className := getNodeText(nameNode, content)
	symbol.Name = className
	symbol.QualifiedName = className
	symbol.ID = GenerateSymbolID(LangPy, filename, symbol.StartLine, className)
	symbol.Signature = "class " + className

	return symbol
}

func extractPythonParameters(paramNode *sitter.Node, content []byte) []Param {
	var params []Param

	for i := uint(0); i < paramNode.ChildCount(); i++ {
		child := paramNode.Child(i)
		// Skip parentheses and commas
		if child.Kind() == "identifier" {
			paramText := getNodeText(child, content)
			params = append(params, Param{
				Name:    paramText,
				Type:    "",
				Snippet: paramText,
			})
		} else if child.Kind() == "typed_parameter" || child.Kind() == "default_parameter" ||
			child.Kind() == "typed_default_parameter" {
			paramText := getNodeText(child, content)
			// Try to extract name
			identifier := findChildByType(child, "identifier")
			paramName := ""
			if identifier != nil {
				paramName = getNodeText(identifier, content)
			}
			params = append(params, Param{
				Name:    paramName,
				Type:    "",
				Snippet: paramText,
			})
		}
	}

	return params
}

func findPythonFunctionCalls(node *sitter.Node, content []byte) []Callee {
	var callees []Callee

	// Recursively search for function calls
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "call" {
			callee := analyzePythonFunctionCall(child, content)
			if callee != nil {
				callees = append(callees, *callee)
			}
		}
		// Recurse into child nodes
		callees = append(callees, findPythonFunctionCalls(child, content)...)
	}

	return callees
}

func analyzePythonFunctionCall(node *sitter.Node, content []byte) *Callee {
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
	snippet := getNodeText(node, content) // Default to just the call expression

	// Try to find the parent statement node
	parent := node.Parent()
	for parent != nil {
		parentKind := parent.Kind()
		if parentKind == "expression_statement" ||
			parentKind == "assignment" ||
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
