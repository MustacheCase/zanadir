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

var rootCmd = &cobra.Command{
	Use:   "zanadir",
	Short: "Grades whether a repository's CI actually protects it",
	Long:  "zanadir reads a repository's CI configuration and reports which security and quality controls are missing, or wired so they cannot fail a build.",
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scans a repository's CI configuration",
	Long:  "The scan command reads a repository's CI configuration and reports the categories it does not cover.",
	Run:   run(config.CreateConfig, scanRepo),
}

var fixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Prints CI configuration for the categories a scan reports as missing",
	Long:  "The fix command prints ready-to-paste CI configuration for each uncovered category, so a scan result can be acted on without leaving the tool.",
	Run:   run(config.CreateFixConfig, fixRepo),
}

// exit is os.Exit, replaced in tests.
var exit = os.Exit

// run builds a command's configuration and does its work, exiting non-zero
// on any failure.
func run(build func(*cobra.Command) (*config.Config, error), do func(*config.Config) error) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, _ []string) {
		cfg, err := build(cmd)
		if err != nil {
			fmt.Printf("Error: Unable to initialize configuration service %v\n", err)
			exit(1)
			return
		}
		if err := do(cfg); err != nil {
			w, msg := scanErrorReport(err)
			_, _ = fmt.Fprint(w, msg)
			exit(1)
		}
	}
}

// addSharedFlags defines the flags scan and fix both take.
func addSharedFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("dir", "d", "", "Path to the repository directory (required)")
	cmd.Flags().StringSliceP("excluded-categories", "e", []string{}, "List of excluded categories (optional)")
	cmd.Flags().Bool("debug", false, "Run the tool using debug mode (optional)")
	cmd.Flags().StringP("config", "c", "", "Read flag values from this YAML file; flags given on the command line win (optional)")
	_ = cmd.MarkFlagRequired("dir")
}

// NewApp initializes the CLI application
func NewApp() *cobra.Command {
	rootCmd.AddCommand(scanCmd, fixCmd)

	addSharedFlags(scanCmd)
	scanCmd.Flags().Bool("enforce", false, "Fails the CI process when any category is uncovered (optional)")
	scanCmd.Flags().StringSlice("fail-on", []string{}, "Fail only when these specific categories are uncovered (optional)")
	scanCmd.Flags().Bool("fail-on-weakened", false, "Also fail when a category has tooling that cannot fail a build (optional)")
	scanCmd.Flags().String("baseline", "", "Path to a baseline file of already-accepted gaps (optional)")
	scanCmd.Flags().Bool("write-baseline", false, "Write the current uncovered categories to the baseline file and exit successfully (optional)")
	scanCmd.Flags().StringP("output", "o", "table", "Output format of the tool (table, json, sarif, markdown) (optional)")
	scanCmd.Flags().String("output-file", "", "Write the report to this file instead of stdout (optional)")
	scanCmd.Flags().String("badge", "", "Write a shields.io endpoint badge of the coverage score to this path (optional)")

	addSharedFlags(fixCmd)
	fixCmd.Flags().Bool("write", false, "Generate .github/workflows/zanadir-suggested.yml instead of printing (optional)")

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
	return os.Stdout, fmt.Sprintf("Error: scan repo failed: %v\n", err)
}

func scanRepo(cfg *config.Config) error {
	h, err := handler.Setup(cfg.Rules...)
	if err != nil {
		return err
	}
	return h.Execute(cfg)
}

func fixRepo(cfg *config.Config) error {
	h, err := handler.Setup(cfg.Rules...)
	if err != nil {
		return err
	}
	return h.Fix(cfg, os.Stdout)
}
