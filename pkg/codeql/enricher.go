package codeql

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/noperator/slice/pkg/analysis"
	"github.com/noperator/slice/pkg/logging"
	"github.com/noperator/slice/pkg/parser"
)

// QueryEnricher handles enriching CodeQL results with source code context
type QueryEnricher struct {
	sourceDir string
	logger    *slog.Logger
}

// NewQueryEnricher creates a new query enricher
func NewQueryEnricher(sourceDir string) *QueryEnricher {
	return &QueryEnricher{
		sourceDir: sourceDir,
		logger:    logging.NewLoggerFromEnv(),
	}
}

// EnrichResults enriches CodeQL results with source code and validation using parallel processing
func (e *QueryEnricher) EnrichResults(results []CodeQLResult, callGraph *analysis.CallGraph, validateCalls bool, callDepth int, concurrency int) ([]Finding, error) {
	// Use atomic counters for thread-safe statistics
	var validationStats struct {
		total   atomic.Int32
		valid   atomic.Int32
		invalid atomic.Int32
	}

	// Determine number of workers
	numWorkers := concurrency
	if numWorkers <= 0 {
		// Auto-detect based on CPU cores if not specified
		numWorkers = runtime.NumCPU()
		if numWorkers > 16 {
			numWorkers = 16
		}
	}
	e.logger.Info("processing results in parallel",
		"component", "codeql",
		"workers", numWorkers,
		"total_results", len(results))

	// Create channels for work distribution
	type workItem struct {
		index  int
		result CodeQLResult
	}
	
	type workResult struct {
		index   int
		finding *Finding // nil if invalid/filtered
		err     error
	}

	workChan := make(chan workItem, len(results))
	resultChan := make(chan workResult, len(results))

	// Create wait group for workers
	var wg sync.WaitGroup

	// Start worker goroutines
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for item := range workChan {
				// Process each result
				finding, err := e.enrichWithSourceCode(item.result)
				if err != nil {
					e.logger.Warn("failed to enrich result with source code",
						"component", "codeql",
						"worker", workerID,
						"result_index", item.index+1,
						"error", err)
					// Still include the result but with empty source code
					finding = Finding{
						CodeQLResult: item.result,
						SourceCode:   SourceCode{},
					}
				}

				// Perform call chain validation if enabled
				includeResult := true
				if validateCalls && callGraph != nil {
					// Use callDepth for search, but default to 10 if -1 (no limit for filtering)
					searchDepth := callDepth
					if searchDepth < 0 {
						searchDepth = 10
					}
					validation := callGraph.ValidateCallRelationship(item.result.FreeFunctionName, item.result.UseFunctionName, searchDepth)
					finding.CallValidation = validation
					
					validationStats.total.Add(1)
					if validation.IsValid {
						// Apply call depth filtering if specified (when callDepth >= 0)
						if callDepth >= 0 && validation.MaxDepth > callDepth {
							validationStats.invalid.Add(1)
							includeResult = false
						} else {
							validationStats.valid.Add(1)
							
							// Populate intermediate functions from call chains
							intermediateFuncs := e.extractIntermediateFunctions(validation.CallChains, item.result.FreeFunctionName, item.result.UseFunctionName)
							for _, funcName := range intermediateFuncs {
								// Try to find the function definition
								funcCode, err := e.findFunctionByName(funcName)
								if err != nil {
									e.logger.Debug("could not find intermediate function",
										"component", "codeql",
										"worker", workerID,
										"function", funcName,
										"error", err)
									// Add empty function code as placeholder
									finding.SourceCode.IntermediateFunctions = append(finding.SourceCode.IntermediateFunctions, 
										FunctionCode{
											DefinitionWithLineNumbers: fmt.Sprintf("// Function %s not found", funcName),
											Snippet: "",
										})
								} else {
									finding.SourceCode.IntermediateFunctions = append(finding.SourceCode.IntermediateFunctions, funcCode)
								}
							}
						}
					} else {
						validationStats.invalid.Add(1)
						includeResult = false
					}
				}
				
				// Send result
				if includeResult {
					resultChan <- workResult{
						index:   item.index,
						finding: &finding,
						err:     nil,
					}
				} else {
					resultChan <- workResult{
						index:   item.index,
						finding: nil,
						err:     nil,
					}
				}
				
				// Log progress periodically
				if (item.index+1)%100 == 0 {
					e.logger.Debug("processing progress",
						"component", "codeql",
						"worker", workerID,
						"processed", item.index+1,
						"total", len(results))
				}
			}
		}(w)
	}

	// Send work items to workers
	for i, result := range results {
		workChan <- workItem{index: i, result: result}
	}
	close(workChan)

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results (maintaining order)
	findingsMap := make(map[int]*Finding)
	for result := range resultChan {
		if result.finding != nil {
			findingsMap[result.index] = result.finding
		}
	}

	// Build ordered results slice
	var enrichedResults []Finding
	for i := 0; i < len(results); i++ {
		if finding, exists := findingsMap[i]; exists {
			enrichedResults = append(enrichedResults, *finding)
		}
	}

	// Print validation statistics
	if validateCalls {
		total := validationStats.total.Load()
		valid := validationStats.valid.Load()
		invalid := validationStats.invalid.Load()
		
		validationRate := 0.0
		if total > 0 {
			validationRate = float64(valid) / float64(total) * 100
		}
		e.logger.Info("call chain validation statistics",
			"component", "codeql",
			"total_findings", total,
			"valid_relationships", valid,
			"invalid_relationships", invalid,
			"validation_rate_percent", validationRate)
	}

	return enrichedResults, nil
}

// enrichWithSourceCode enriches a CodeQL result with source code context
func (e *QueryEnricher) enrichWithSourceCode(result CodeQLResult) (Finding, error) {
	// Create function IDs for free and use functions with full paths
	freeID := fmt.Sprintf("%s:%d:%s", filepath.Join(e.sourceDir, result.FreeFunctionFile), result.FreeFunctionDefLine, result.FreeFunctionName)
	useID := fmt.Sprintf("%s:%d:%s", filepath.Join(e.sourceDir, result.UseFunctionFile), result.UseFunctionDefLine, result.UseFunctionName)
	
	// Find functions using parser
	freeFunc, err := parser.FindFunctionByID(e.sourceDir, freeID)
	if err != nil {
		return Finding{}, fmt.Errorf("failed to find free function %s: %w", freeID, err)
	}
	
	useFunc, err := parser.FindFunctionByID(e.sourceDir, useID)
	if err != nil {
		return Finding{}, fmt.Errorf("failed to find use function %s: %w", useID, err)
	}
	
	// Get specific line snippets
	freeSnippet, err := e.getLineFromFile(filepath.Join(e.sourceDir, result.FreeFunctionFile), result.FreeLine)
	if err != nil {
		freeSnippet = fmt.Sprintf("// Could not retrieve line %d: %v", result.FreeLine, err)
	}
	
	useSnippet, err := e.getLineFromFile(filepath.Join(e.sourceDir, result.UseFunctionFile), result.UseLine)
	if err != nil {
		useSnippet = fmt.Sprintf("// Could not retrieve line %d: %v", result.UseLine, err)
	}
	
	// Create the finding (without call chain - that's in call_validation now)
	finding := Finding{
		CodeQLResult: result,
		SourceCode: SourceCode{
			FreeFunction: FunctionCode{
				DefinitionWithLineNumbers: freeFunc.DefinitionWithLineNumbers,
				Snippet:                  freeSnippet,
			},
			UseFunction: FunctionCode{
				DefinitionWithLineNumbers: useFunc.DefinitionWithLineNumbers,
				Snippet:                  useSnippet,
			},
			IntermediateFunctions: []FunctionCode{}, // Will be populated after validation
		},
	}
	
	return finding, nil
}

// getLineFromFile retrieves a specific line from a file
func (e *QueryEnricher) getLineFromFile(filePath string, lineNum int) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	currentLine := 1
	
	for scanner.Scan() {
		if currentLine == lineNum {
			return strings.TrimSpace(scanner.Text()), nil
		}
		currentLine++
	}
	
	if err := scanner.Err(); err != nil {
		return "", err
	}
	
	return "", fmt.Errorf("line %d not found in file %s", lineNum, filePath)
}

// extractIntermediateFunctions finds functions that appear in call chains between free and use functions
func (e *QueryEnricher) extractIntermediateFunctions(callChains [][]string, freeFunc, useFunc string) []string {
	intermediateMap := make(map[string]bool)
	
	for _, chain := range callChains {
		for _, funcName := range chain {
			// Skip the free and use functions themselves
			if funcName != freeFunc && funcName != useFunc {
				intermediateMap[funcName] = true
			}
		}
	}
	
	// Convert map to slice
	var intermediates []string
	for funcName := range intermediateMap {
		intermediates = append(intermediates, funcName)
	}
	
	return intermediates
}

// findFunctionByName searches for a function definition by name across all files
func (e *QueryEnricher) findFunctionByName(funcName string) (FunctionCode, error) {
	// Get cached analysis result
	analysisResult, err := parser.GetCachedAnalysisResult(e.sourceDir)
	if err != nil {
		return FunctionCode{}, fmt.Errorf("failed to get cached analysis: %w", err)
	}
	
	// Search for the function by name
	for _, function := range analysisResult.Symbols {
		if function.Name == funcName {
			// Found the function, now get its full definition
			funcID := function.ID
			fullFunc, err := parser.FindFunctionByID(e.sourceDir, funcID)
			if err != nil {
				// Try to return what we have
				return FunctionCode{
					DefinitionWithLineNumbers: function.DefinitionWithLineNumbers,
					Snippet:                  "",
				}, nil
			}
			
			return FunctionCode{
				DefinitionWithLineNumbers: fullFunc.DefinitionWithLineNumbers,
				Snippet:                  "", // We don't have a specific line for intermediate functions
			}, nil
		}
	}
	
	return FunctionCode{}, fmt.Errorf("function %s not found in codebase", funcName)
}