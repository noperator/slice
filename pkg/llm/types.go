package llm

import (
	"encoding/json"
	"strings"

	"github.com/noperator/slice/pkg/analysis"
	"github.com/noperator/slice/pkg/codeql"
)


// CodeQLRequest contains the data needed for LLM processing of CodeQL findings
type CodeQLRequest struct {
	CodeQLResult         codeql.CodeQLResult           `json:"codeql_result"`
	SourceCode           codeql.SourceCode             `json:"source_code"`
	CallValidation       *analysis.CallValidation      `json:"call_validation,omitempty"`
	FreeFuncDef          string                        `json:"free_function_definition"`
	UseFuncDef           string                        `json:"use_function_definition"`
	IntermediateFuncDefs []string                      `json:"intermediate_function_definitions"`
	CallChains           [][]string                    `json:"chains"`
	FreeSnippet          string                        `json:"free_snippet"`
	UseSnippet           string                        `json:"use_snippet"`
}

// UnifiedResult represents a finding that can be progressively enriched
type UnifiedResult struct {
	// Base finding from CodeQL (present when from CodeQL query)
	CodeQLResult *codeql.CodeQLResult `json:"query,omitempty"`
	SourceCode   *codeql.SourceCode   `json:"source,omitempty"`

	// Optional call validation results (present when --validate-calls is enabled)
	CallValidation *analysis.CallValidation `json:"calls,omitempty"`

	// Optional ranking results (present after rank command)
	Rank *RankInfo `json:"rank,omitempty"`

	// Raw data for tool-agnostic input (JSONL mode or other tools)
	RawData map[string]interface{} `json:"raw_data,omitempty"`

	// Dynamic results with custom keys (for template-defined output keys)
	DynamicResults map[string]interface{} `json:"-"`

	// Store the template type for this result (used during processing)
	templateType string `json:"-"`
}

// MarshalJSON implements custom JSON marshaling for UnifiedResult
func (ur *UnifiedResult) MarshalJSON() ([]byte, error) {
	// Start with the basic struct fields
	type Alias UnifiedResult
	aux := &struct {
		*Alias
		DynamicResults map[string]interface{} `json:"-"`
		TemplateType   string                 `json:"-"`
	}{
		Alias: (*Alias)(ur),
	}

	// Marshal the basic struct first
	basicJSON, err := json.Marshal(aux)
	if err != nil {
		return nil, err
	}

	// If no dynamic results, return the basic JSON
	if len(ur.DynamicResults) == 0 {
		return basicJSON, nil
	}

	// Parse the basic JSON into a map
	var result map[string]interface{}
	if err := json.Unmarshal(basicJSON, &result); err != nil {
		return nil, err
	}

	// Add dynamic results to the map
	for key, value := range ur.DynamicResults {
		result[key] = value
	}

	// Marshal the combined result
	return json.Marshal(result)
}

// UnmarshalJSON implements custom JSON unmarshaling for UnifiedResult
func (ur *UnifiedResult) UnmarshalJSON(data []byte) error {
	// First unmarshal into a map to separate known and unknown fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Initialize dynamic results map
	if ur.DynamicResults == nil {
		ur.DynamicResults = make(map[string]interface{})
	}

	// Known field names that should be handled by regular struct unmarshaling
	knownFields := map[string]bool{
		"query":    true,
		"source":   true,
		"calls":    true,
		"rank":     true,
		"raw_data": true,
	}

	// Separate known and dynamic fields
	knownFieldsData := make(map[string]json.RawMessage)
	for key, value := range raw {
		if knownFields[key] {
			knownFieldsData[key] = value
		} else {
			// This is a dynamic field - unmarshal it
			var dynamicValue interface{}
			if err := json.Unmarshal(value, &dynamicValue); err != nil {
				return err
			}
			ur.DynamicResults[key] = dynamicValue
		}
	}

	// Marshal and unmarshal the known fields into the struct
	if len(knownFieldsData) > 0 {
		knownJSON, err := json.Marshal(knownFieldsData)
		if err != nil {
			return err
		}

		type Alias UnifiedResult
		aux := &struct {
			*Alias
		}{
			Alias: (*Alias)(ur),
		}

		if err := json.Unmarshal(knownJSON, aux); err != nil {
			return err
		}
	}

	return nil
}

// SetDynamicResult sets a dynamic result with the specified key
func (ur *UnifiedResult) SetDynamicResult(key string, value interface{}) {
	if ur.DynamicResults == nil {
		ur.DynamicResults = make(map[string]interface{})
	}
	ur.DynamicResults[key] = value
}

// GetDynamicResult gets a dynamic result by key
func (ur *UnifiedResult) GetDynamicResult(key string) (interface{}, bool) {
	if ur.DynamicResults == nil {
		return nil, false
	}
	value, exists := ur.DynamicResults[key]
	return value, exists
}

// UnifiedOutput represents the standard output format for all commands
type UnifiedOutput struct {
	QueryFile string          `json:"query_file"`
	Database  string          `json:"codeql_db"`
	SrcDir    string          `json:"src_dir,omitempty"`
	Results   []UnifiedResult `json:"results"`
}



// TokenUsage represents token usage statistics from an API call
type TokenUsage struct {
	Timestamp                string `json:"timestamp"`
	Model                    string `json:"model"`
	FunctionContext          string `json:"function_context"` // template type
	PromptTokens             int64  `json:"prompt_tokens"`
	CompletionTokens         int64  `json:"completion_tokens"`
	ReasoningTokens          int64  `json:"reasoning_tokens,omitempty"`
	AudioTokens              int64  `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens int64  `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int64  `json:"rejected_prediction_tokens,omitempty"`
	TotalTokens              int64  `json:"total_tokens"`
	ReasoningEffort          string `json:"reasoning_effort,omitempty"`
	ResponseID               string `json:"response_id"`
	// Cost estimation
	InputCostUSD  float64 `json:"input_cost_usd"`
	OutputCostUSD float64 `json:"output_cost_usd"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
}

// TokenStats tracks cumulative token usage across all API calls
type TokenStats struct {
	TotalPromptTokens     int64 `json:"total_prompt_tokens"`
	TotalCompletionTokens int64 `json:"total_completion_tokens"`
	TotalReasoningTokens  int64 `json:"total_reasoning_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
	CallCount             int64 `json:"call_count"`
	// Cost tracking
	TotalInputCostUSD  float64 `json:"total_input_cost_usd"`
	TotalOutputCostUSD float64 `json:"total_output_cost_usd"`
	TotalCostUSD       float64 `json:"total_cost_usd"`
}

// ModelPricing represents the pricing structure for a model
type ModelPricing struct {
	InputPerMillion       float64 // USD per 1M input tokens
	CachedInputPerMillion float64 // USD per 1M cached input tokens (if supported)
	OutputPerMillion      float64 // USD per 1M output tokens
}

// GetModelPricing returns pricing information for known models
func GetModelPricing(model string) *ModelPricing {
	// Normalize model name to handle variants like "openai/gpt-5", "gpt-5", etc.
	normalizedModel := strings.ToLower(model)
	normalizedModel = strings.TrimPrefix(normalizedModel, "openai/")

	switch {
	// GPT-5 series
	case strings.Contains(normalizedModel, "gpt-5-nano") || strings.Contains(normalizedModel, "gpt5-nano"):
		return &ModelPricing{
			InputPerMillion:       0.050,
			CachedInputPerMillion: 0.005,
			OutputPerMillion:      0.400,
		}
	case strings.Contains(normalizedModel, "gpt-5-mini") || strings.Contains(normalizedModel, "gpt5-mini"):
		return &ModelPricing{
			InputPerMillion:       0.250,
			CachedInputPerMillion: 0.025,
			OutputPerMillion:      2.000,
		}
	case strings.Contains(normalizedModel, "gpt-5") || strings.Contains(normalizedModel, "gpt5"):
		return &ModelPricing{
			InputPerMillion:       1.250,
			CachedInputPerMillion: 0.125,
			OutputPerMillion:      10.000,
		}

	default:
		// Return nil for unknown models - no cost estimation
		return nil
	}
}

// CalculateCost estimates the cost of a token usage
func (tu *TokenUsage) CalculateCost() {
	pricing := GetModelPricing(tu.Model)
	if pricing == nil {
		// Unknown model, can't calculate cost
		tu.InputCostUSD = 0
		tu.OutputCostUSD = 0
		tu.TotalCostUSD = 0
		return
	}

	// Calculate input cost (prompt tokens)
	tu.InputCostUSD = float64(tu.PromptTokens) * pricing.InputPerMillion / 1_000_000

	// Calculate output cost (completion tokens + reasoning tokens)
	// For reasoning models, reasoning tokens are billed as output tokens
	outputTokens := tu.CompletionTokens + tu.ReasoningTokens
	tu.OutputCostUSD = float64(outputTokens) * pricing.OutputPerMillion / 1_000_000

	// Total cost
	tu.TotalCostUSD = tu.InputCostUSD + tu.OutputCostUSD
}

// Config holds the configuration for LLM analysis
type Config struct {
	APIKey          string  `json:"api_key"`
	BaseURL         string  `json:"base_url"`         // For OpenAI-compatible APIs
	Model           string  `json:"model"`            // Model to use (e.g., "gpt-4", "gpt-3.5-turbo")
	Temperature     float32 `json:"temperature"`      // Temperature for response generation
	MaxTokens       int     `json:"max_tokens"`       // Maximum tokens in response
	ReasoningEffort string  `json:"reasoning_effort"` // Reasoning effort for GPT-5: minimal, low, medium, high
	PromptTemplate  string  `json:"prompt_template"`  // Path to custom prompt template file
}

// RankInfo contains ranking information from raink
type RankInfo struct {
	Score    float64 `json:"score"`    // Ranking score from raink
	Exposure int     `json:"exposure"` // How many times seen during ranking
	Pos      int     `json:"pos"`      // 1-based rank position (1 = highest priority)
}
