package main

import (
	"encoding/json"

	"github.com/noperator/slice/pkg/relate"
	"github.com/spf13/cobra"
)

var relateCmd = &cobra.Command{
	Use:   "relate",
	Short: "Join saved line candidates by function call relationships",
	Long: `Join two saved lines collections using functions from saved parse output.
Matches include the same enclosing function or a call chain in either direction.
Call connectivity does not establish statement order, object identity, or vulnerability validity.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		parsed, _ := cmd.Flags().GetString("parsed")
		left, _ := cmd.Flags().GetString("left")
		right, _ := cmd.Flags().GetString("right")
		relationship, _ := cmd.Flags().GetString("relationship")
		depth, _ := cmd.Flags().GetInt("call-depth")
		result, err := relate.Run(parsed, left, right, relationship, depth, cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	},
}

func init() {
	relateCmd.Flags().String("parsed", "", "Saved parse JSON containing functions (required)")
	relateCmd.Flags().String("left", "", "Left candidate JSON containing lines (required)")
	relateCmd.Flags().String("right", "", "Right candidate JSON containing lines (required)")
	relateCmd.Flags().String("relationship", "", "Relationship to match: call (required)")
	relateCmd.Flags().Int("call-depth", 3, "Maximum call edges (nonnegative)")
	for _, flag := range []string{"parsed", "left", "right", "relationship"} {
		_ = relateCmd.MarkFlagRequired(flag)
	}
	rootCmd.AddCommand(relateCmd)
}
