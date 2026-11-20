package main

import (
	"encoding/json"

	"github.com/noperator/slice/pkg/parser"
	"github.com/spf13/cobra"
)

var parseCmd = &cobra.Command{
	Use:   "parse <directory>",
	Short: "Extract functions and source lines from C files",
	Long: `Recursively extract functions and physical source lines from regular .c and .h files.
By default only functions are included. Use --lines for lines only, or --functions --lines for both collections.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		functions, _ := cmd.Flags().GetBool("functions")
		lines, _ := cmd.Flags().GetBool("lines")
		if !functions && !lines {
			functions = true
		}
		result, err := parser.ExtractDirectory(args[0], functions, lines)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	},
}

func init() {
	parseCmd.Flags().Bool("functions", false, "Extract function records")
	parseCmd.Flags().Bool("lines", false, "Extract every physical source line")
	rootCmd.AddCommand(parseCmd)
}
