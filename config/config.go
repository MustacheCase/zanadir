package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/baseline"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/spf13/cobra"
)

const (
	OutputJSON     = "json"
	OutputTable    = "table"
	OutputSARIF    = "sarif"
	OutputMarkdown = "markdown"
)

// OutputFormats is every value --output accepts, in the order they are listed
// to the user.
var OutputFormats = []string{OutputTable, OutputJSON, OutputSARIF, OutputMarkdown}

type Config struct {
	Dir                string
	ExcludedCategories []string
	Enforce            bool
	// FailOn limits enforcement to specific categories; empty means all.
	FailOn []string
	// FailOnWeakened makes a control that cannot fail a build fail the scan.
	FailOnWeakened bool
	// Baseline is the path to a file of already-accepted gaps.
	Baseline      string
	WriteBaseline bool
	Debug         bool
	Output        string
	// OutputFile writes the report to a path instead of stdout; empty means stdout.
	OutputFile string
	// Badge writes a shields.io endpoint badge to a path; empty means no badge.
	Badge string
	// Write makes the fix command generate a workflow instead of printing.
	Write bool
	// Rules are custom rules from the config file, added to the catalogue.
	Rules []rules.FileRule
}

// normalizeCategories canonicalises category names and rejects unknown ones.
func normalizeCategories(categories []string) ([]string, error) {
	normalized := make([]string, 0, len(categories))
	for _, c := range categories {
		title, ok := models.ResolveCategory(c)
		if !ok {
			return nil, fmt.Errorf("unknown category %q: valid categories are %s", c, strings.Join(models.CategoryNames(), ", "))
		}
		normalized = append(normalized, string(title))
	}
	return normalized, nil
}

func resolveDir(cmd *cobra.Command) (string, error) {
	dir, _ := cmd.Flags().GetString("dir")
	if dir == "" {
		_ = cmd.Help()
		return "", fmt.Errorf("error: --dir (-d) flag is required")
	}

	dir = filepath.Clean(dir)

	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("error: Symlinks are not allowed")
	}

	return dir, nil
}

// categoriesFlag reads a list of category names and canonicalises it.
func categoriesFlag(cmd *cobra.Command, name string) ([]string, error) {
	categories, _ := cmd.Flags().GetStringSlice(name)
	return normalizeCategories(categories)
}

// newConfig reads what scan and fix share: the directory, the config file
// with any custom rules, the exclusions and debug.
func newConfig(cmd *cobra.Command) (*Config, error) {
	dir, err := resolveDir(cmd)
	if err != nil {
		return nil, err
	}
	file, err := applyFile(cmd)
	if err != nil {
		return nil, err
	}
	excluded, err := categoriesFlag(cmd, "excluded-categories")
	if err != nil {
		return nil, err
	}
	debug, _ := cmd.Flags().GetBool("debug")
	return &Config{Dir: dir, ExcludedCategories: excluded, Debug: debug, Rules: file.Rules}, nil
}

func CreateConfig(cmd *cobra.Command) (*Config, error) {
	cfg, err := newConfig(cmd)
	if err != nil {
		return nil, err
	}
	if cfg.FailOn, err = categoriesFlag(cmd, "fail-on"); err != nil {
		return nil, err
	}

	flags := cmd.Flags()
	cfg.Enforce, _ = flags.GetBool("enforce")
	cfg.FailOnWeakened, _ = flags.GetBool("fail-on-weakened")
	cfg.Baseline, _ = flags.GetString("baseline")
	cfg.WriteBaseline, _ = flags.GetBool("write-baseline")
	cfg.Badge, _ = flags.GetString("badge")
	cfg.OutputFile, _ = flags.GetString("output-file")
	cfg.Output, _ = flags.GetString("output")

	// --write-baseline with no --baseline writes to the default location.
	if cfg.WriteBaseline && cfg.Baseline == "" {
		cfg.Baseline = baseline.DefaultPath
	}
	if !slices.Contains(OutputFormats, cfg.Output) {
		return nil, fmt.Errorf("unsupported output format: %s (expected one of %s)", cfg.Output, strings.Join(OutputFormats, ", "))
	}
	return cfg, nil
}

// CreateFixConfig builds the subset of configuration the fix command needs.
// It does not read the scan-only flags, which fix does not define.
func CreateFixConfig(cmd *cobra.Command) (*Config, error) {
	cfg, err := newConfig(cmd)
	if err != nil {
		return nil, err
	}
	cfg.Write, _ = cmd.Flags().GetBool("write")
	return cfg, nil
}
