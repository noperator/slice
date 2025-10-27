package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/noperator/slice/pkg/logging"
)

// Pipeline handles the complete LLM processing pipeline for vulnerability analysis
type Pipeline struct {
	analyzer  *Analyzer
	config    PipelineConfig
	logger    *slog.Logger
	outputAll bool
}

// PipelineConfig contains configuration for the processing pipeline
type PipelineConfig struct {
	Timeout        time.Duration
	Concurrency    int
	PromptTemplate string
	OutputAll      bool
}

// NewPipeline creates a new LLM processing pipeline
func NewPipeline(analyzerConfig Config, pipelineConfig PipelineConfig) *Pipeline {
	return &Pipeline{
		analyzer:  NewAnalyzer(analyzerConfig),
		config:    pipelineConfig,
		logger:    logging.NewLoggerFromEnv(),
		outputAll: pipelineConfig.OutputAll,
	}
}

// ProcessResults processes unified results using the template-driven approach
func (p *Pipeline) ProcessResults(ctx context.Context, input *UnifiedOutput) (*UnifiedOutput, error) {
	metadata, err := p.getTemplateMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to get template metadata: %w", err)
	}

	timeout := p.config.Timeout
	if metadata.Timeout > 0 {
		timeout = time.Duration(metadata.Timeout) * time.Second
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	outputResults, err := p.processWithTemplate(timeoutCtx, input, metadata)
	if err != nil {
		return nil, err
	}

	// Apply final filtering based on is_valid field unless --all flag is set
	if !p.outputAll {
		filteredResults := p.filterResultsByValidity(outputResults.Results, metadata)
		originalCount := len(outputResults.Results)
		outputResults.Results = filteredResults

		if originalCount != len(filteredResults) {
			p.logger.Info("filtered results by validity",
				"original_count", originalCount,
				"valid_count", len(filteredResults),
				"filtered_out", originalCount-len(filteredResults))
		}
	}

	// Print summary and token statistics
	p.printSummaryAndStats(outputResults, metadata)

	return outputResults, nil
}

// ReadInputResults reads unified results from file or stdin
func (p *Pipeline) ReadInputResults(inputFile string, jsonlMode bool) (*UnifiedOutput, error) {
	var inputResults *UnifiedOutput
	var err error

	if jsonlMode {
		inputResults, err = p.readJSONLInput(inputFile)
	} else {
		if inputFile != "" {
			inputResults, err = ReadUnifiedResultsFromFile(inputFile)
		} else {
			inputResults, err = ReadUnifiedResultsFromStdin()
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read input results: %w", err)
	}

	if len(inputResults.Results) == 0 {
		p.logger.Info("no vulnerabilities found to process")
		return inputResults, nil
	}

	return inputResults, nil
}

// readJSONLInput reads JSONL format (one JSON object per line)
func (p *Pipeline) readJSONLInput(inputFile string) (*UnifiedOutput, error) {
	var reader io.Reader
	var file *os.File
	var err error

	if inputFile != "" {
		file, err = os.Open(inputFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open input file: %w", err)
		}
		defer file.Close()
		reader = file
	} else {
		reader = os.Stdin
	}

	var results []UnifiedResult
	scanner := bufio.NewScanner(reader)

	// Increase buffer size for large lines
	const maxCapacity = 1024 * 1024 // 1MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rawData map[string]interface{}
		if err := json.Unmarshal(line, &rawData); err != nil {
			return nil, fmt.Errorf("failed to parse JSON on line %d: %w", lineNum, err)
		}

		// Create UnifiedResult with raw data
		results = append(results, UnifiedResult{
			RawData: rawData,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	return &UnifiedOutput{
		Results: results,
	}, nil
}

// WriteOutputResults writes unified results to file or stdout
func (p *Pipeline) WriteOutputResults(results *UnifiedOutput, outputFile string) error {
	var err error
	if outputFile != "" {
		err = WriteUnifiedResultsToFile(results, outputFile)
	} else {
		err = WriteUnifiedResultsToStdout(results)
	}

	if err != nil {
		return fmt.Errorf("failed to write results: %w", err)
	}

	return nil
}

// LoadEnvironmentConfig loads configuration from environment variables
func (p *Pipeline) LoadEnvironmentConfig(config *Config) error {
	if config.APIKey == "" {
		config.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	if config.BaseURL == "" {
		config.BaseURL = os.Getenv("OPENAI_API_BASE")
	}
	if config.Model == "gpt-4" { // Only override default if still default
		if envModel := os.Getenv("OPENAI_API_MODEL"); envModel != "" {
			config.Model = envModel
		}
	}

	if config.APIKey == "" {
		return fmt.Errorf("API key is required (set OPENAI_API_KEY environment variable)")
	}

	return nil
}

// filterResultsByValidity filters results based on their valid field
func (p *Pipeline) filterResultsByValidity(results []UnifiedResult, metadata *TemplateMetadata) []UnifiedResult {
	var filteredResults []UnifiedResult

	for _, result := range results {
		var isValid bool

		// Check if result has a "valid" field in dynamic results
		if dynamicResult, exists := result.GetDynamicResult(metadata.Type); exists {
			if resultMap, ok := dynamicResult.(map[string]interface{}); ok {
				if validValue, hasValid := resultMap["valid"]; hasValid {
					if validBool, ok := validValue.(bool); ok {
						isValid = validBool
					}
				}
			}
		}

		if isValid {
			filteredResults = append(filteredResults, result)
		}
	}

	return filteredResults
}

// getTemplateMetadata gets template metadata or returns defaults
func (p *Pipeline) getTemplateMetadata() (*TemplateMetadata, error) {
	if p.config.PromptTemplate == "" {
		return &TemplateMetadata{Type: "generic"}, nil
	}

	return ParseTemplateMetadata(p.config.PromptTemplate)
}

// processWithTemplate performs unified processing using the specified template
func (p *Pipeline) processWithTemplate(ctx context.Context, input *UnifiedOutput, metadata *TemplateMetadata) (*UnifiedOutput, error) {
	p.logger.Info("processing findings",
		"component", "analyzer",
		"operation", metadata.Type,
		"findings", len(input.Results),
		"model", p.analyzer.config.Model,
		"concurrency", p.config.Concurrency)

	return p.processWithWorkerPool(ctx, input, metadata.Type, p.createUnifiedProcessor(metadata))
}

// processWithWorkerPool processes results using a worker pool
func (p *Pipeline) processWithWorkerPool(ctx context.Context, input *UnifiedOutput,
	operationName string, processor ProcessFunc[UnifiedResult, UnifiedResult]) (*UnifiedOutput, error) {

	concurrency := p.config.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	// Create worker pool
	pool := NewWorkerPool[UnifiedResult, UnifiedResult](concurrency)

	// Process all results
	results, err := pool.ProcessItems(ctx, input.Results, processor, operationName)
	if err != nil {
		return nil, fmt.Errorf("%s processing failed: %w", operationName, err)
	}

	return &UnifiedOutput{
		QueryFile: input.QueryFile,
		Database:  input.Database,
		SrcDir:    input.SrcDir,
		Results:   results,
	}, nil
}

// createUnifiedProcessor creates a processor function for any template type
func (p *Pipeline) createUnifiedProcessor(metadata *TemplateMetadata) ProcessFunc[UnifiedResult, UnifiedResult] {
	return ProcessFunc[UnifiedResult, UnifiedResult](func(ctx context.Context, result UnifiedResult) (UnifiedResult, error) {
		// Check if already processed
		if _, exists := result.GetDynamicResult(metadata.Type); exists {
			return result, nil
		}

		// Pass the result directly to template - no transformation
		response, err := p.analyzer.ProcessFinding(ctx, result, p.config.PromptTemplate)
		if err != nil {
			p.logger.Warn("failed to process finding",
				"component", "analyzer",
				"template_type", metadata.Type,
				"error", err)

			// Generic fallback for any template type
			response = map[string]interface{}{
				"valid": false,
				"error": fmt.Sprintf("Processing failed: %v", err),
			}
		}

		// Store result using dynamic key
		result.SetDynamicResult(metadata.Type, response)

		return result, nil
	})
}

// printSummaryAndStats prints summary and token usage statistics
func (p *Pipeline) printSummaryAndStats(outputResults *UnifiedOutput, metadata *TemplateMetadata) {
	// Print generic summary
	valid := 0
	for _, result := range outputResults.Results {
		if dynamicResult, exists := result.GetDynamicResult(metadata.Type); exists {
			if resultMap, ok := dynamicResult.(map[string]interface{}); ok {
				if validValue, hasValid := resultMap["valid"]; hasValid {
					if validBool, ok := validValue.(bool); ok && validBool {
						valid++
					}
				}
			}
		}
	}

	p.logger.Info("processing complete",
		"template_type", metadata.Type,
		"results_processed", len(outputResults.Results),
		"valid_results", valid,
		"invalid_results", len(outputResults.Results)-valid)

	p.printTokenStats()
}

// printTokenStats prints token usage statistics
func (p *Pipeline) printTokenStats() {
	stats := p.analyzer.GetTokenStats()
	if stats.TotalCostUSD > 0 {
		p.logger.Info("token usage statistics",
			"component", "analyzer",
			"total_calls", stats.CallCount,
			"prompt_tokens", stats.TotalPromptTokens,
			"completion_tokens", stats.TotalCompletionTokens,
			"reasoning_tokens", stats.TotalReasoningTokens,
			"total_tokens", stats.TotalTokens,
			"cost_usd", stats.TotalCostUSD)
	} else {
		p.logger.Info("token usage statistics",
			"component", "analyzer",
			"total_calls", stats.CallCount,
			"prompt_tokens", stats.TotalPromptTokens,
			"completion_tokens", stats.TotalCompletionTokens,
			"reasoning_tokens", stats.TotalReasoningTokens,
			"total_tokens", stats.TotalTokens)
	}
}
