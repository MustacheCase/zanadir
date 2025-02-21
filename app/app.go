package app

import (
	"fmt"
	"os"

	"github.com/MustacheCase/zanadir/handler"
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
		if err := scanRepo(dir); err != nil {
			os.Exit(1)
		}
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

// scanRepo function
func scanRepo(dir string) error {
	scanHandler, err := handler.Setup()
	if err != nil {
		// log the error
		return err
	}
	// Add scanning logic here
	err = scanHandler.Execute(dir)
	if err != nil {
		return err
	}

	return nil
}
