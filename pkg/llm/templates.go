package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// CodeQLTemplateData holds the data for CodeQL template rendering
type codeQLTemplateData struct {
	ObjectName           string
	FreeFunctionName     string
	FreeFunctionFile     string
	FreeLine             int
	UseFunctionName      string
	UseFunctionFile      string
	UseLine              int
	CallChain            []string   // For backward compatibility
	CallChains           [][]string // Multiple call chains
	FreeSnippet          string
	UseSnippet           string
	FreeFunctionDef      string
	UseFunctionDef       string
	IntermediateFuncDefs []string
	SchemaJSON           string // Pretty-printed JSON schema for insertion into template
}

// RenderTemplate renders a template with any data structure
// The data is passed directly to the template - no transformation is performed
func RenderTemplate(data interface{}, customTemplatePath string) (string, error) {
	if customTemplatePath == "" {
		return "", fmt.Errorf("template path is required - no default template available")
	}

	content, err := os.ReadFile(customTemplatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template file %s: %w", customTemplatePath, err)
	}
	templateContent := string(content)

	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}

	tmpl, err := template.New("template").Funcs(funcMap).Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var result bytes.Buffer
	if err := tmpl.Execute(&result, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return result.String(), nil
}

// TemplateMetadata holds parsed template metadata
type TemplateMetadata struct {
	Type        string                 `json:"type"`
	Schema      map[string]interface{} `json:"schema"`
	Timeout     int                    `json:"timeout"`
	MaxTokens   int                    `json:"max_tokens"`
	Temperature float32                `json:"temperature"`
}

// ParseTemplateMetadata parses template metadata into a structured format
func ParseTemplateMetadata(templatePath string) (*TemplateMetadata, error) {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	// Parse metadata from comments
	metadata := &TemplateMetadata{
		Type: "generic", // default
	}

	var schemaFile string

	// Look for metadata in first few lines
	lines := strings.Split(string(content), "\n")
	for i := 0; i < len(lines) && i < 15; i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{{/*") && strings.HasSuffix(line, "*/}}") {
			// Extract content between {{/* and */}}
			commentContent := line[4 : len(line)-4]
			commentContent = strings.TrimSpace(commentContent)

			// Handle simple key-value pairs
			if colonIndex := strings.Index(commentContent, ":"); colonIndex > 0 {
				key := strings.TrimSpace(commentContent[:colonIndex])
				value := strings.TrimSpace(commentContent[colonIndex+1:])

				switch key {
				case "type":
					metadata.Type = value
				case "schema_file":
					schemaFile = value
				case "schema":
					// Legacy: handle inline schema for backwards compatibility
					if schemaFile == "" {
						if schema, err := parseJSONSchema(value); err == nil {
							metadata.Schema = schema
						}
					}
				case "timeout":
					if timeoutVal := parseInt(value); timeoutVal > 0 {
						metadata.Timeout = timeoutVal
					}
				case "max_tokens":
					if tokensVal := parseInt(value); tokensVal > 0 {
						metadata.MaxTokens = tokensVal
					}
				case "temperature":
					if tempVal := parseFloat32(value); tempVal >= 0 {
						metadata.Temperature = tempVal
					}
				}
			}
		}
	}

	// Load schema from external file if specified
	if schemaFile != "" {
		schemaPath := schemaFile
		// If it's a relative path, make it relative to the template file's directory
		if !filepath.IsAbs(schemaFile) {
			templateDir := filepath.Dir(templatePath)
			schemaPath = filepath.Join(templateDir, schemaFile)
		}

		schemaContent, err := os.ReadFile(schemaPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read schema file %s: %w", schemaPath, err)
		}

		if schema, err := parseJSONSchema(string(schemaContent)); err == nil {
			metadata.Schema = schema
		} else {
			return nil, fmt.Errorf("failed to parse schema file %s: %w", schemaPath, err)
		}
	}

	return metadata, nil
}

// extractSchemaFromComment extracts a multi-line JSON schema from template comments
func extractSchemaFromComment(lines []string, startIndex int) string {
	var schemaLines []string
	found := false

	for i := startIndex; i < len(lines) && i < startIndex+10; i++ {
		line := lines[i]

		// Look for schema: marker
		if strings.Contains(line, "schema:") {
			// Extract everything after "schema:"
			if idx := strings.Index(line, "schema:"); idx >= 0 {
				after := line[idx+7:] // 7 = len("schema:")
				// Remove comment markers
				after = strings.TrimPrefix(after, "{{/*")
				after = strings.TrimSuffix(after, "*/}}")
				after = strings.TrimSpace(after)
				if after != "" {
					schemaLines = append(schemaLines, after)
				}
				found = true
				continue
			}
		}

		// If we found the start, collect subsequent lines until we hit */}} or a new {{/*
		if found {
			if strings.Contains(line, "*/}}") {
				// This line contains the end marker
				if idx := strings.Index(line, "*/}}"); idx >= 0 {
					part := strings.TrimSpace(line[:idx])
					if part != "" {
						schemaLines = append(schemaLines, part)
					}
				}
				break
			} else if strings.Contains(line, "{{/*") {
				// Hit a new comment, stop
				break
			} else {
				// Regular line, add it
				schemaLines = append(schemaLines, strings.TrimSpace(line))
			}
		}
	}

	return strings.Join(schemaLines, " ")
}

// parseJSONSchema parses a JSON schema string
func parseJSONSchema(schemaStr string) (map[string]interface{}, error) {
	var schema map[string]interface{}
	err := json.Unmarshal([]byte(schemaStr), &schema)
	return schema, err
}

// parseInt safely parses an integer from string
func parseInt(s string) int {
	val := 0
	if n, _ := fmt.Sscanf(s, "%d", &val); n == 1 {
		return val
	}
	return 0
}

// parseFloat32 safely parses a float32 from string
func parseFloat32(s string) float32 {
	val := float32(0)
	if n, _ := fmt.Sscanf(s, "%f", &val); n == 1 {
		return val
	}
	return -1
}

// convertSchemaToExample converts a JSON schema to an example representation
func convertSchemaToExample(schema map[string]interface{}) interface{} {
	schemaType, _ := schema["type"].(string)

	switch schemaType {
	case "object":
		result := make(map[string]interface{})
		if properties, ok := schema["properties"].(map[string]interface{}); ok {
			for key, prop := range properties {
				if propMap, ok := prop.(map[string]interface{}); ok {
					result[key] = convertSchemaToExample(propMap)
				}
			}
		}
		return result

	case "array":
		if items, ok := schema["items"].(map[string]interface{}); ok {
			return []interface{}{convertSchemaToExample(items)}
		}
		return []interface{}{"string"}

	case "string":
		if enum, ok := schema["enum"].([]interface{}); ok && len(enum) > 0 {
			// For enums, show all options separated by |
			var options []string
			for _, e := range enum {
				if s, ok := e.(string); ok {
					options = append(options, s)
				}
			}
			if len(options) > 0 {
				return strings.Join(options, "|")
			}
		}
		return "string"

	case "number", "integer":
		return "number"

	case "boolean":
		return "boolean"

	default:
		return "unknown"
	}
}
