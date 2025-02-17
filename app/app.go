package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Root command (CLI entry point)
var rootCmd = &cobra.Command{
	Use:   "zanadir",
	Short: "zanadir CLI tool",
	Long:  "zanadir is a CLI tool that provides which provides suggestions how to improve your CI.",
}

// scanCmd represents the "scan" command
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scans a GitHub repository directory",
	Long:  "The scan command scans a specified GitHub repository directory for CI analysis.",
	Run: func(cmd *cobra.Command, args []string) {
		dir, _ := cmd.Flags().GetString("dir")
		if dir == "" {
			fmt.Println("Error: --dir (-d) flag is required")
			cmd.Help()
			os.Exit(1)
		}
		scanRepo(dir)
	},
}

// NewApp initializes the CLI application
func NewApp() *cobra.Command {
	// Add scan command to root command
	rootCmd.AddCommand(scanCmd)

	// Add flags to scan command
	scanCmd.Flags().StringP("dir", "d", "", "Path to the GitHub repository directory (required)")
	_ = scanCmd.MarkFlagRequired("dir")

	return rootCmd
}

// scanRepo function (dummy implementation)
func scanRepo(dir string) {
	// Add scanning logic here
}
