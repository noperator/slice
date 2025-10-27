package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/noperator/slice/pkg/llm"
)

var (
	llmBaseURL         string
	llmModel           string
	llmTemp            float32
	llmMaxTokens       int
	llmReasoningEffort string
	promptTemplate     string
	inputFile          string
	outputFile         string
	timeout            int
	concurrency        int
	outputAll          bool
	jsonlIn            bool
)


var filterCmd = &cobra.Command{
	Use:   "filter [flags]",
	Short: "Filter CodeQL vulnerability results using LLM processing",
	Long: `Filter CodeQL vulnerability detection results using a Large Language Model.

This command processes vulnerability findings using a template-driven approach.
The template determines the behavior, output structure, and processing parameters.

Template metadata can specify custom behavior via comments:
  {{/* type: custom_type */}}
  {{/* timeout: 180 */}}
  {{/* max_tokens: 32000 */}}
  {{/* schema: {"type": "object", "properties": {"valid": {"type": "boolean"}}} */}}

By default, only valid/vulnerable results are output. Use --all to output everything.

Examples:
  # Process with custom template
  slice query --database db --query uaf.ql --source /path/to/src | slice filter -p spec/uaf/custom.tmpl --model gpt-4

  # Process with different template
  slice filter --input results.json -p spec/uaf/detailed.tmpl --model gpt-4

  # Use custom template with embedded schema
  slice filter --input query-results.json -p custom-template.tmpl
  
  # Output all results including invalid ones
  slice filter --all --input results.json -p spec/uaf/detailed.tmpl --model gpt-4`,
	RunE: func(cmd *cobra.Command, args []string) error {
		processorConfig := llm.Config{
			APIKey:          "",
			BaseURL:         llmBaseURL,
			Model:           llmModel,
			Temperature:     llmTemp,
			MaxTokens:       llmMaxTokens,
			ReasoningEffort: llmReasoningEffort,
			PromptTemplate:  promptTemplate,
		}

		pipeline := &llm.Pipeline{}
		if err := pipeline.LoadEnvironmentConfig(&processorConfig); err != nil {
			return err
		}

		pipelineConfig := llm.PipelineConfig{
			Timeout:        time.Duration(timeout) * time.Second,
			Concurrency:    concurrency,
			PromptTemplate: promptTemplate,
			OutputAll:      outputAll,
		}


		pipeline = llm.NewPipeline(processorConfig, pipelineConfig)

		inputResults, err := pipeline.ReadInputResults(inputFile, jsonlIn)
		if err != nil {
			return err
		}

		if len(inputResults.Results) == 0 {
			return nil
		}

		outputResults, err := pipeline.ProcessResults(context.Background(), inputResults)
		if err != nil {
			return fmt.Errorf("processing failed: %w", err)
		}

		return pipeline.WriteOutputResults(outputResults, outputFile)
	},
}

func init() {
	filterCmd.Flags().StringVarP(&llmBaseURL, "base-url", "b", "", "Base URL for OpenAI-compatible API (optional, or set OPENAI_API_BASE env var)")
	filterCmd.Flags().StringVarP(&llmModel, "model", "m", "gpt-4", "Model to use (or set OPENAI_API_MODEL env var)")
	filterCmd.Flags().Float32Var(&llmTemp, "temperature", 0.1, "Temperature for response generation")
	filterCmd.Flags().IntVarP(&llmMaxTokens, "max-tokens", "t", 64000, "Maximum tokens in response")
	filterCmd.Flags().StringVarP(&llmReasoningEffort, "reasoning-effort", "r", "high", "Reasoning effort for GPT-5 models: minimal, low, medium, high")

	filterCmd.Flags().StringVarP(&inputFile, "input", "i", "", "Input file containing CodeQL query results (if not provided, reads from stdin)")
	filterCmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file for filter results (if not provided, writes to stdout)")

	filterCmd.Flags().IntVar(&timeout, "timeout", 300, "Timeout in seconds (adjusted automatically based on template type)")
	filterCmd.Flags().IntVarP(&concurrency, "concurrency", "j", 10, "Number of concurrent LLM API calls")

	filterCmd.Flags().StringVarP(&promptTemplate, "prompt-template", "p", "", 
		"Path to custom prompt template file (optional)")

	filterCmd.Flags().BoolVarP(&outputAll, "all", "a", false,
		"Output all results regardless of validity (default: only output valid/vulnerable results)")

	filterCmd.Flags().BoolVar(&jsonlIn, "jsonl-in", false,
		"Read input as JSONL (one JSON object per line) instead of wrapped JSON with results key")

	rootCmd.AddCommand(filterCmd)
}


