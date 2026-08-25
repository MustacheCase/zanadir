package app

import (
	"errors"
	"fmt"
	"io"
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
			w, msg := scanErrorReport(err)
			_, _ = fmt.Fprint(w, msg)
			os.Exit(1)
		}
	},
}

var fixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Prints CI configuration for the categories a scan reports as missing",
	Long:  "The fix command prints ready-to-paste CI configuration for each uncovered category, so a scan result can be acted on without leaving the tool.",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.CreateFixConfig(cmd)
		if err != nil {
			fmt.Printf("Error: Unable to initialize configuration service %v", err)
			os.Exit(1)
		}

		if err := fixRepo(cfg); err != nil {
			w, msg := scanErrorReport(err)
			_, _ = fmt.Fprint(w, msg)
			os.Exit(1)
		}
	},
}

// NewApp initializes the CLI application
func NewApp() *cobra.Command {
	// Add scan command to root command
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(fixCmd)

	// Add flags to scan command
	scanCmd.Flags().StringP("dir", "d", "", "Path to the GitHub repository directory (required)")
	scanCmd.Flags().StringSliceP("excluded-categories", "e", []string{}, "List of excluded categories (optional)")
	scanCmd.Flags().Bool("enforce", false, "Fails the CI process when any category is uncovered (optional)")
	scanCmd.Flags().StringSlice("fail-on", []string{}, "Fail only when these specific categories are uncovered (optional)")
	scanCmd.Flags().String("baseline", "", "Path to a baseline file of already-accepted gaps (optional)")
	scanCmd.Flags().Bool("write-baseline", false, "Write the current uncovered categories to the baseline file and exit successfully (optional)")
	scanCmd.Flags().Bool("debug", false, "Run the tool using debug mode (optional)")
	scanCmd.Flags().StringP("output", "o", "table", "Output format of the tool (table, json, sarif) (optional)")
	scanCmd.Flags().String("output-file", "", "Write the report to this file instead of stdout (optional)")

	_ = scanCmd.MarkFlagRequired("dir")

	fixCmd.Flags().StringP("dir", "d", "", "Path to the GitHub repository directory (required)")
	fixCmd.Flags().StringSliceP("excluded-categories", "e", []string{}, "List of excluded categories (optional)")
	fixCmd.Flags().Bool("debug", false, "Run the tool using debug mode (optional)")
	_ = fixCmd.MarkFlagRequired("dir")

	return rootCmd
}

// scanErrorReport decides how a failed scan is reported. An enforcement failure
// is the tool doing its job, so it names the categories on stderr — not obvious
// once --fail-on or a baseline narrows enforcement down. Anything else is an
// operational error and keeps its original destination.
func scanErrorReport(err error) (io.Writer, string) {
	var enforceErr *models.EnforceError
	if errors.As(err, &enforceErr) {
		return os.Stderr, fmt.Sprintf("Enforcement failed: %v\n", err)
	}
	return os.Stdout, fmt.Sprintf("Error: scan repo failed: %v", err)
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

func fixRepo(cfg *config.Config) error {
	fixHandler, err := handler.Setup()
	if err != nil {
		return err
	}
	return fixHandler.Fix(cfg, os.Stdout)
}
