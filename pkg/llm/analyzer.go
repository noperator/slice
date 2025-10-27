package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/noperator/slice/pkg/logging"
)

// Template-driven schema generation
// Schemas are generated dynamically based on template metadata

// Analyzer handles LLM-based analysis with structured JSON output
type Analyzer struct {
	client openai.Client
	config Config
	logger *slog.Logger
	tokenStats TokenStats
	statsMutex sync.Mutex
}

// NewAnalyzer creates a new LLM analyzer
func NewAnalyzer(config Config) *Analyzer {
	opts := []option.RequestOption{
		option.WithAPIKey(config.APIKey),
	}
	
	if config.BaseURL != "" {
		baseURL := config.BaseURL
		if !strings.HasSuffix(baseURL, "/") {
			baseURL += "/"
		}
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	
	client := openai.NewClient(opts...)
	
	return &Analyzer{
		client: client,
		config: config,
		logger: logging.NewLoggerFromEnv(),
	}
}


// logTokenUsage logs token usage to stderr and updates statistics
func (a *Analyzer) logTokenUsage(usage TokenUsage) {
	a.statsMutex.Lock()
	a.tokenStats.TotalPromptTokens += usage.PromptTokens
	a.tokenStats.TotalCompletionTokens += usage.CompletionTokens
	a.tokenStats.TotalReasoningTokens += usage.ReasoningTokens
	a.tokenStats.TotalTokens += usage.TotalTokens
	a.tokenStats.CallCount++
	a.tokenStats.TotalInputCostUSD += usage.InputCostUSD
	a.tokenStats.TotalOutputCostUSD += usage.OutputCostUSD
	a.tokenStats.TotalCostUSD += usage.TotalCostUSD
	// Track template type usage
	// Note: FunctionContext now contains template type instead of hardcoded modes
	a.statsMutex.Unlock()
	
	// Always log to stderr
	totalInput := usage.PromptTokens
	totalOutput := usage.CompletionTokens + usage.ReasoningTokens
	if usage.TotalCostUSD > 0 {
		a.logger.Debug("token usage",
			"component", "analyzer",
			"timestamp", usage.Timestamp,
			"model", usage.Model,
			"function_context", usage.FunctionContext,
			"input_tokens", totalInput,
			"output_tokens", totalOutput,
			"completion_tokens", usage.CompletionTokens,
			"reasoning_tokens", usage.ReasoningTokens,
			"input_cost_usd", usage.InputCostUSD,
			"output_cost_usd", usage.OutputCostUSD,
			"total_cost_usd", usage.TotalCostUSD)
	} else {
		a.logger.Debug("token usage",
			"component", "analyzer",
			"timestamp", usage.Timestamp,
			"model", usage.Model,
			"function_context", usage.FunctionContext,
			"input_tokens", totalInput,
			"output_tokens", totalOutput,
			"completion_tokens", usage.CompletionTokens,
			"reasoning_tokens", usage.ReasoningTokens)
	}
}


// extractTokenUsage extracts token usage from OpenAI response
func (a *Analyzer) extractTokenUsage(resp *openai.ChatCompletion, functionContext string) TokenUsage {
	usage := TokenUsage{
		Timestamp:       time.Now().Format(time.RFC3339),
		Model:           resp.Model,
		FunctionContext: functionContext,
		PromptTokens:    resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:     resp.Usage.TotalTokens,
		ReasoningEffort: a.config.ReasoningEffort,
		ResponseID:      resp.ID,
	}
	
	// Debug logging for token details when enabled
	if os.Getenv("SLICE_DEBUG_TOKENS") == "1" {
		a.logger.Debug("token usage details",
			"component", "analyzer",
			"model", resp.Model,
			"reasoning_effort_set", a.config.ReasoningEffort,
			"reasoning_tokens", resp.Usage.CompletionTokensDetails.ReasoningTokens,
			"audio_tokens", resp.Usage.CompletionTokensDetails.AudioTokens,
			"accepted_prediction_tokens", resp.Usage.CompletionTokensDetails.AcceptedPredictionTokens,
			"rejected_prediction_tokens", resp.Usage.CompletionTokensDetails.RejectedPredictionTokens,
			"raw_usage", fmt.Sprintf("%+v", resp.Usage))
	}
	
	// Extract detailed completion token information
	if resp.Usage.CompletionTokensDetails.ReasoningTokens > 0 {
		usage.ReasoningTokens = resp.Usage.CompletionTokensDetails.ReasoningTokens
	}
	if resp.Usage.CompletionTokensDetails.AudioTokens > 0 {
		usage.AudioTokens = resp.Usage.CompletionTokensDetails.AudioTokens
	}
	if resp.Usage.CompletionTokensDetails.AcceptedPredictionTokens > 0 {
		usage.AcceptedPredictionTokens = resp.Usage.CompletionTokensDetails.AcceptedPredictionTokens
	}
	if resp.Usage.CompletionTokensDetails.RejectedPredictionTokens > 0 {
		usage.RejectedPredictionTokens = resp.Usage.CompletionTokensDetails.RejectedPredictionTokens
	}
	
	// Calculate cost estimation
	usage.CalculateCost()
	
	return usage
}

// GetTokenStats returns current token usage statistics
func (a *Analyzer) GetTokenStats() TokenStats {
	a.statsMutex.Lock()
	defer a.statsMutex.Unlock()
	return a.tokenStats
}

// ProcessFinding processes a finding using the specified template
// The finding can be any data structure (UnifiedResult, map[string]interface{}, etc.)
func (a *Analyzer) ProcessFinding(ctx context.Context, finding interface{}, templatePath string) (interface{}, error) {
	// Parse template metadata
	metadata, err := ParseTemplateMetadata(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template metadata: %w", err)
	}

	// Render the prompt using the template
	prompt, err := RenderTemplate(finding, templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to render prompt: %w", err)
	}

	// Make the API call with template-defined schema
	result, err := a.callLLMWithMetadata(ctx, prompt, metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM (prompt length: %d chars): %w", len(prompt), err)
	}

	return result, nil
}

// callLLMWithMetadata makes the API call using template-defined metadata and schema
func (a *Analyzer) callLLMWithMetadata(ctx context.Context, prompt string, metadata *TemplateMetadata) (interface{}, error) {
	// Debug logging for prompt content (helpful for diagnosing refusals)
	if os.Getenv("SLICE_DEBUG_PROMPTS") == "1" {
		fmt.Fprintf(os.Stderr, "=== PROMPT DEBUG (%s) ===\n%s\n=== END PROMPT ===\n", metadata.Type, prompt)
	}

	// Templates must define their own schema
	if metadata.Schema == nil {
		return nil, fmt.Errorf("template must define a schema - no schema found in template metadata")
	}
	
	// Use template-embedded schema
	schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:        fmt.Sprintf("%s_response", metadata.Type),
		Description: openai.String(fmt.Sprintf("Response for %s template", metadata.Type)),
		Schema:      metadata.Schema,
		Strict:      openai.Bool(true),
	}
	systemMessage := "You are a security expert. Provide your response in the exact structured format specified by the template."

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemMessage),
			openai.UserMessage(prompt),
		},
		Model: openai.ChatModel(a.config.Model),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: schemaParam,
			},
		},
	}

	// Use template-defined parameters or config defaults
	maxTokens := a.config.MaxTokens
	if metadata.MaxTokens > 0 {
		maxTokens = metadata.MaxTokens
	}
	
	temperature := a.config.Temperature
	if metadata.Temperature >= 0 {
		temperature = metadata.Temperature
	}

	// Set model-specific parameters based on model type
	modelName := string(a.config.Model)
	isGPT5 := strings.Contains(modelName, "gpt-5") || strings.Contains(modelName, "gpt5")
	
	if isGPT5 {
		// GPT-5 specific parameters
		params.MaxCompletionTokens = openai.Int(int64(maxTokens))
		// GPT-5 only supports default temperature (1.0), so don't set it
	} else {
		// Standard models
		params.MaxTokens = openai.Int(int64(maxTokens))
		params.Temperature = openai.Float(float64(temperature))
	}

	// Add reasoning effort for o-series and GPT-5 models if configured
	if (strings.Contains(modelName, "o1") || strings.Contains(modelName, "o3") || strings.Contains(modelName, "o4") || 
		strings.Contains(modelName, "gpt-5") || strings.Contains(modelName, "gpt5")) && a.config.ReasoningEffort != "" {
		params.ReasoningEffort = openai.ReasoningEffort(a.config.ReasoningEffort)
		
		// Debug logging for reasoning effort
		if os.Getenv("SLICE_DEBUG_TOKENS") == "1" {
			fmt.Fprintf(os.Stderr, "=== REASONING EFFORT DEBUG ===\nModel: %s\nReasoning Effort: %s\nApplied: true\n=== END REASONING DEBUG ===\n",
				modelName, a.config.ReasoningEffort)
		}
	} else if os.Getenv("SLICE_DEBUG_TOKENS") == "1" {
		fmt.Fprintf(os.Stderr, "=== REASONING EFFORT DEBUG ===\nModel: %s\nReasoning Effort: %s\nApplied: false (model not supported or effort empty)\n=== END REASONING DEBUG ===\n",
			modelName, a.config.ReasoningEffort)
	}

	resp, err := a.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("OpenAI API call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response choices returned from LLM")
	}

	content := resp.Choices[0].Message.Content
	
	// Debug: Log what we actually received if debug mode is on
	if os.Getenv("SLICE_DEBUG_PROMPTS") == "1" {
		fmt.Fprintf(os.Stderr, "=== RESPONSE DEBUG (%s) ===\nContent length: %d\nContent: %q\nFinish reason: %v\nCompletion tokens: %d\n=== END RESPONSE ===\n", 
			metadata.Type, len(content), content, resp.Choices[0].FinishReason, resp.Usage.CompletionTokens)
	}
	
	// Check for empty content (could indicate refusal or other issues)
	if content == "" {
		choice := resp.Choices[0]
		var refusalInfo string
		if choice.Message.Refusal != "" {
			refusalInfo = fmt.Sprintf(", refusal: %s", choice.Message.Refusal)
		}
		
		// Include comprehensive debugging info
		usageInfo := ""
		if resp.Usage.TotalTokens > 0 {
			usageInfo = fmt.Sprintf(", usage: {prompt_tokens: %d, completion_tokens: %d, total_tokens: %d}", 
				resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
		}
		
		return nil, fmt.Errorf("LLM returned empty content - this may indicate a refusal, content policy violation, or API issue. Finish reason: %v%s%s. Model: %s, Response ID: %s", 
			choice.FinishReason, refusalInfo, usageInfo, resp.Model, resp.ID)
	}
	
	// Parse the structured JSON response as generic map
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, a.formatParseError(content, err, resp, metadata.Type)
	}
	
	// Log token usage
	tokenUsage := a.extractTokenUsage(resp, metadata.Type)
	a.logTokenUsage(tokenUsage)
	
	return result, nil
}

// formatParseError formats JSON parse error with debugging info
func (a *Analyzer) formatParseError(content string, err error, resp *openai.ChatCompletion, templateType string) error {
	choice := resp.Choices[0]
	usageInfo := ""
	if resp.Usage.TotalTokens > 0 {
		usageInfo = fmt.Sprintf(", usage: {prompt_tokens: %d, completion_tokens: %d, total_tokens: %d}", 
			resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}
	return fmt.Errorf("failed to parse %s JSON response (content: %q): %w. Finish reason: %v%s. Model: %s, Response ID: %s", 
		templateType, content, err, choice.FinishReason, usageInfo, resp.Model, resp.ID)
}


