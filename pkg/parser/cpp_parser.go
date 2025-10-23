package parser

import (
	"fmt"
	"os"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

func parseCppFile(filename string) ([]Symbol, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	parser := sitter.NewParser()
	language := sitter.NewLanguage(tree_sitter_cpp.Language())
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
	symbols = append(symbols, findCppFunctions(root, content, filename)...)
	symbols = append(symbols, findCppClasses(root, content, filename)...)

	return symbols, nil
}

func findCppFunctions(node *sitter.Node, content []byte, filename string) []Symbol {
	var symbols []Symbol

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "function_definition" {
			symbol := analyzeCppFunction(child, content, filename)
			if symbol != nil {
				symbols = append(symbols, *symbol)
			}
		}
		// Recurse but skip class bodies
		if child.Kind() != "class_specifier" {
			symbols = append(symbols, findCppFunctions(child, content, filename)...)
		}
	}

	return symbols
}

func findCppClasses(node *sitter.Node, content []byte, filename string) []Symbol {
	var symbols []Symbol

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "class_specifier" {
			// Add the class itself
			classSymbol := analyzeCppClass(child, content, filename)
			if classSymbol != nil {
				symbols = append(symbols, *classSymbol)

				// Find methods within this class
				className := classSymbol.Name
				methods := findCppMethods(child, content, filename, className)
				symbols = append(symbols, methods...)
			}
		}
		// Recurse to find nested classes
		symbols = append(symbols, findCppClasses(child, content, filename)...)
	}

	return symbols
}

func findCppMethods(classNode *sitter.Node, content []byte, filename string, className string) []Symbol {
	var symbols []Symbol

	// Look for field_declaration_list (class body)
	for i := uint(0); i < classNode.ChildCount(); i++ {
		child := classNode.Child(i)
		if child.Kind() == "field_declaration_list" {
			// Find function_definitions within the class body
			for j := uint(0); j < child.ChildCount(); j++ {
				methodNode := child.Child(j)
				if methodNode.Kind() == "function_definition" {
					method := analyzeCppMethod(methodNode, content, filename, className)
					if method != nil {
						symbols = append(symbols, *method)
					}
				}
			}
		}
	}

	return symbols
}

func analyzeCppFunction(node *sitter.Node, content []byte, filename string) *Symbol {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	defText := getNodeText(node, content)
	symbol := &Symbol{
		Lang:                      LangCpp,
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

	// Extract function signature and parameters
	declarator := findCppDeclarator(node)
	if declarator == nil {
		return nil
	}

	// Check if this is a method defined outside a class (has qualified_identifier)
	qualifiedID := findChildByType(declarator, "qualified_identifier")
	if qualifiedID != nil {
		// This is a method defined outside the class body
		className, methodName := extractQualifiedIdentifierParts(qualifiedID, content)

		if methodName != "" {
			symbol.Kind = "method"
			symbol.Name = methodName
			symbol.QualifiedName = className + "::" + methodName
			symbol.Metadata = make(map[string]interface{})
			symbol.Metadata["class_name"] = className

			// Generate method ID with language prefix
			symbol.ID = GenerateSymbolID(LangCpp, filename, symbol.StartLine, symbol.QualifiedName)

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
				symbol.Params = extractCppParameters(paramList, content)
			}

			// Build full signature
			var paramStrings []string
			for _, param := range symbol.Params {
				paramStrings = append(paramStrings, param.Snippet)
			}
			symbol.Signature = strings.TrimSpace(returnType) + " " + symbol.QualifiedName + "(" + strings.Join(paramStrings, ", ") + ")"
		}
	} else {
		// Regular function (not a method)
		identifier := findCppFunctionIdentifier(declarator, content)
		if identifier != "" {
			functionName := identifier
			symbol.Name = functionName
			symbol.QualifiedName = functionName

			// Generate function ID with language prefix
			symbol.ID = GenerateSymbolID(LangCpp, filename, symbol.StartLine, functionName)

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
				symbol.Params = extractCppParameters(paramList, content)
			}

			// Build full signature
			var paramStrings []string
			for _, param := range symbol.Params {
				paramStrings = append(paramStrings, param.Snippet)
			}
			symbol.Signature = strings.TrimSpace(returnType) + " " + functionName + "(" + strings.Join(paramStrings, ", ") + ")"
		}
	}

	// Find function body
	body := findChildByType(node, "compound_statement")
	if body != nil {
		// Extract function calls
		symbol.Callees = findCppFunctionCalls(body, content)

		// Extract variables
		symbol.Vars = findCppVariables(body, content, symbol.Params)
	}

	return symbol
}

func analyzeCppMethod(node *sitter.Node, content []byte, filename string, className string) *Symbol {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	defText := getNodeText(node, content)
	symbol := &Symbol{
		Lang:                      LangCpp,
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

	// Extract method signature and parameters
	declarator := findCppDeclarator(node)
	if declarator == nil {
		return nil
	}

	// Get method name
	identifier := findCppFunctionIdentifier(declarator, content)
	if identifier != "" {
		methodName := identifier
		symbol.Name = methodName
		symbol.QualifiedName = className + "::" + methodName

		// Generate method ID with language prefix
		symbol.ID = GenerateSymbolID(LangCpp, filename, symbol.StartLine, symbol.QualifiedName)

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
			symbol.Params = extractCppParameters(paramList, content)
		}

		// Build full signature
		var paramStrings []string
		for _, param := range symbol.Params {
			paramStrings = append(paramStrings, param.Snippet)
		}
		symbol.Signature = strings.TrimSpace(returnType) + " " + className + "::" + methodName + "(" + strings.Join(paramStrings, ", ") + ")"
	}

	// Find method body
	body := findChildByType(node, "compound_statement")
	if body != nil {
		// Extract function calls
		symbol.Callees = findCppFunctionCalls(body, content)

		// Extract variables
		symbol.Vars = findCppVariables(body, content, symbol.Params)
	}

	return symbol
}

func analyzeCppClass(node *sitter.Node, content []byte, filename string) *Symbol {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	defText := getNodeText(node, content)
	symbol := &Symbol{
		Lang:                      LangCpp,
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
	var className string
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "type_identifier" {
			className = getNodeText(child, content)
			break
		}
	}

	if className != "" {
		symbol.Name = className
		symbol.QualifiedName = className
		symbol.ID = GenerateSymbolID(LangCpp, filename, symbol.StartLine, className)
		symbol.Signature = "class " + className
	} else {
		return nil
	}

	return symbol
}

func findCppDeclarator(node *sitter.Node) *sitter.Node {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "function_declarator" {
			return child
		}
	}
	return nil
}

func findCppFunctionIdentifier(declarator *sitter.Node, content []byte) string {
	for i := uint(0); i < declarator.ChildCount(); i++ {
		child := declarator.Child(i)
		if child.Kind() == "identifier" || child.Kind() == "field_identifier" {
			return getNodeText(child, content)
		} else if child.Kind() == "qualified_identifier" {
			// Handle qualified identifiers like ClassName::methodName
			// Extract just the method name (the part after ::)
			qualifiedText := getNodeText(child, content)
			parts := strings.Split(qualifiedText, "::")
			if len(parts) > 0 {
				return parts[len(parts)-1] // Return the last part (method name)
			}
		} else if child.Kind() == "destructor_name" {
			// Handle destructors like ~ClassName()
			return getNodeText(child, content)
		} else if child.Kind() == "operator_name" {
			// Handle operator overloads like operator==, operator!=, etc.
			return getNodeText(child, content)
		}
	}
	return ""
}

// extractQualifiedIdentifierParts extracts class name and method name from a qualified_identifier node
func extractQualifiedIdentifierParts(qualifiedID *sitter.Node, content []byte) (className string, methodName string) {
	qualifiedText := getNodeText(qualifiedID, content)
	parts := strings.Split(qualifiedText, "::")

	if len(parts) >= 2 {
		// Join all parts except the last as class name (handles nested namespaces)
		className = strings.Join(parts[:len(parts)-1], "::")
		methodName = parts[len(parts)-1]
	} else if len(parts) == 1 {
		methodName = parts[0]
	}

	return className, methodName
}

func extractCppParameters(paramList *sitter.Node, content []byte) []Param {
	var params []Param

	for i := uint(0); i < paramList.ChildCount(); i++ {
		child := paramList.Child(i)
		if child.Kind() == "parameter_declaration" || child.Kind() == "optional_parameter_declaration" {
			paramText := getNodeText(child, content)
			paramText = strings.TrimSpace(paramText)

			// Parse the parameter into components
			param := parseCppParameter(paramText)
			if param != nil {
				params = append(params, *param)
			}
		}
	}

	return params
}

func parseCppParameter(paramText string) *Param {
	if paramText == "" {
		return nil
	}

	// Split the parameter text into words
	words := strings.Fields(paramText)
	if len(words) == 0 {
		return nil
	}

	// The last word (possibly with * or & prefix) is the variable name
	lastWord := words[len(words)-1]

	// Extract the variable name by removing pointer/reference indicators
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

func findCppFunctionCalls(node *sitter.Node, content []byte) []Callee {
	var callees []Callee

	// Recursively search for function calls
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "call_expression" {
			callee := analyzeCppFunctionCall(child, content)
			if callee != nil {
				callees = append(callees, *callee)
			}
		}
		// Recurse into child nodes
		callees = append(callees, findCppFunctionCalls(child, content)...)
	}

	return callees
}

func analyzeCppFunctionCall(node *sitter.Node, content []byte) *Callee {
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

func findCppVariables(node *sitter.Node, content []byte, params []Param) []Variable {
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
	findCppLocalVariableDeclarations(node, content, varMap)

	// Convert map to slice
	var variables []Variable
	for _, v := range varMap {
		variables = append(variables, *v)
	}

	return variables
}

func findCppLocalVariableDeclarations(node *sitter.Node, content []byte, varMap map[string]*Variable) {
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)

		if child.Kind() == "declaration" {
			// Extract basic variable declarations without complex analysis
			extractCppBasicVariableDeclaration(child, content, varMap)
		}

		// Recurse into child nodes
		findCppLocalVariableDeclarations(child, content, varMap)
	}
}

func extractCppBasicVariableDeclaration(node *sitter.Node, content []byte, varMap map[string]*Variable) {
	var typeParts []string
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "primitive_type" || child.Kind() == "type_identifier" ||
			child.Kind() == "class_specifier" || child.Kind() == "storage_class_specifier" {
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
