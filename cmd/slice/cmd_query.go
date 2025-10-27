package main

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/noperator/slice/pkg/analysis"
	"github.com/noperator/slice/pkg/codeql"
	"github.com/noperator/slice/pkg/llm"
	"github.com/noperator/slice/pkg/logging"
	"github.com/noperator/slice/pkg/parser"
)

var (
	database        string
	queryFile       string
	codeqlBin       string
	sourceDir       string
	noValidate      bool
	callDepth       int
	queryConcurrency int
)

var queryLogger *slog.Logger

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Run CodeQL queries and enrich results with source code",
	Long: `Run CodeQL queries against a database and enrich the vulnerability findings 
with full source code context using TreeSitter parsing.

This command integrates CodeQL-based vulnerability detection with the existing 
TreeSitter parsing infrastructure to provide comprehensive vulnerability reports.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		queryLogger = logging.NewLoggerFromEnv()

		if database == "" {
			return fmt.Errorf("database path is required (use --database)")
		}
		if queryFile == "" {
			return fmt.Errorf("query file is required (use --query)")
		}
		if sourceDir == "" {
			return fmt.Errorf("source directory is required (use --source)")
		}

		executor, err := codeql.NewExecutor(codeqlBin)
		if err != nil {
			return fmt.Errorf("failed to initialize CodeQL executor: %w", err)
		}

		if err := executor.CheckCodeQLAvailable(); err != nil {
			return fmt.Errorf("CodeQL not available: %w", err)
		}

		queryLogger.Info("running codeql query",
			"component", "codeql",
			"operation", "query",
			"query_file", queryFile,
			"database", database)

		codeqlResults, err := executor.RunQuery(database, queryFile)
		if err != nil {
			return fmt.Errorf("failed to run CodeQL query: %w", err)
		}

		queryLogger.Info("codeql query complete",
			"component", "codeql",
			"results_found", len(codeqlResults),
			"source_directory", sourceDir)

		var callGraph *analysis.CallGraph
		validateCalls := !noValidate
		if validateCalls {
			queryLogger.Info("building call graph for validation",
				"component", "codeql",
				"operation", "build_call_graph")
			analysisResult, err := parser.GetCachedAnalysisResult(sourceDir)
			if err != nil {
				return fmt.Errorf("failed to parse source code for call graph: %w", err)
			}
			callGraph = analysis.BuildCallGraph(analysisResult.Symbols)
			queryLogger.Info("call graph built",
				"component", "codeql",
				"symbols", len(analysisResult.Symbols))
		}

		enricher := codeql.NewQueryEnricher(sourceDir)
		findings, err := enricher.EnrichResults(codeqlResults, callGraph, validateCalls, callDepth, queryConcurrency)
		if err != nil {
			return fmt.Errorf("failed to enrich query results: %w", err)
		}

		var results []llm.UnifiedResult
		for _, finding := range findings {
			unifiedResult := llm.UnifiedResult{
				CodeQLResult:   &finding.CodeQLResult,
				SourceCode:     &finding.SourceCode,
				CallValidation: finding.CallValidation,
			}
			results = append(results, unifiedResult)
		}

		unifiedOutput := llm.UnifiedOutput{
			QueryFile: queryFile,
			Database:  database,
			SrcDir:    sourceDir,
			Results:   results,
		}

		output, err := json.MarshalIndent(unifiedOutput, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err)
		}

		fmt.Println(string(output))

		queryLogger.Info("query processing complete",
			"component", "codeql",
			"findings_processed", len(results))
		return nil
	},
}



func init() {
	queryCmd.Flags().StringVarP(&database, "database", "d", "", "Path to CodeQL database (required)")
	queryCmd.Flags().StringVarP(&queryFile, "query", "q", "", "Path to CodeQL query file (.ql) (required)")
	queryCmd.Flags().StringVarP(&sourceDir, "source", "s", "", "Path to source code directory (required)")
	queryCmd.Flags().StringVarP(&codeqlBin, "codeql-bin", "b", "", "Path to CodeQL CLI binary (default: resolve from PATH)")
	queryCmd.Flags().BoolVar(&noValidate, "no-validate", false, "Disable call chain validation")
	queryCmd.Flags().IntVarP(&callDepth, "call-depth", "c", -1, "Maximum call chain depth (-1 = no limit)")
	queryCmd.Flags().IntVarP(&queryConcurrency, "concurrency", "j", 0, "Number of concurrent workers for result processing (0 = auto-detect based on CPU cores)")
	
	queryCmd.MarkFlagRequired("database")
	queryCmd.MarkFlagRequired("query")
	queryCmd.MarkFlagRequired("source")
	
	rootCmd.AddCommand(queryCmd)
}