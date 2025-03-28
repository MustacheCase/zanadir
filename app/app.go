package app

import (
	"fmt"
	"os"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/handler"
	"github.com/MustacheCase/zanadir/models"
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
		config, err := config.CreateConfig(cmd)

		if err != nil {
			fmt.Printf("Error: Unable to initialize configuration service %v", err)
			os.Exit(1)
		}

		if err := scanRepo(config); err != nil {
			if _, ok := err.(*models.EnforceError); ok {
				// Do not print the error, just exit
				os.Exit(1)
			}
			fmt.Printf("Error: scan repo failed: %v", err)
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
	scanCmd.Flags().StringSliceP("excluded-categories", "e", []string{}, "List of excluded categories (optional)")
	scanCmd.Flags().Bool("enforce", false, "Fails the CI process when at least one rule is met (optional)")
	_ = scanCmd.MarkFlagRequired("dir")

	return rootCmd
}

// scanRepo function
func scanRepo(config *config.Config) error {
	scanHandler, err := handler.Setup()
	if err != nil {
		// log the error
		return err
	}
	// Add scanning logic here
	err = scanHandler.Execute(config)
	if err != nil {
		return err
	}

	return nil
}
