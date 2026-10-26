// Package config turns command-line flags and an optional config file into a Config.
package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/baseline"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/remote"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/spf13/cobra"
)

// Output formats accepted by --output.
const (
	OutputJSON     = "json"
	OutputTable    = "table"
	OutputSARIF    = "sarif"
	OutputMarkdown = "markdown"
)

// OutputFormats is every value --output accepts, in the order they are listed
// to the user.
var OutputFormats = []string{OutputTable, OutputJSON, OutputSARIF, OutputMarkdown}

// Config is everything a scan or fix run needs, resolved from flags and the config file.
type Config struct {
	Dir                string
	ExcludedCategories []string
	Enforce            bool
	// FailOn limits enforcement to specific categories; empty means all.
	FailOn []string
	// FailOnWeakened makes a control that cannot fail a build fail the scan.
	FailOnWeakened bool
	// RequiredChecks reads the default branch's required checks from GitHub,
	// so a control counts as enforcing only when its check must pass.
	RequiredChecks bool
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
	// Repo is a repository to fetch and scan instead of Dir; nil for a local
	// checkout.
	Repo *remote.Repository
	// Out receives the report instead of OutputFile or stdout, for callers
	// that are not the command line.
	Out io.Writer
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
		return "", fmt.Errorf("error: --dir (-d) or --repo is required")
	}

	// A symlink the operator names is followed; what the scan reads is held
	// inside the directory it points at by repofs.
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("--dir %s is not a directory", dir)
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
	cfg, err := target(cmd)
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
	cfg.ExcludedCategories, cfg.Rules = excluded, file.Rules
	cfg.Debug, _ = cmd.Flags().GetBool("debug")
	return cfg, nil
}

// target reads what to scan: a local checkout with --dir, or a repository to
// fetch with --repo, never both.
func target(cmd *cobra.Command) (*Config, error) {
	repo, _ := cmd.Flags().GetString("repo")
	if repo == "" {
		dir, err := resolveDir(cmd)
		return &Config{Dir: dir}, err
	}
	if dir, _ := cmd.Flags().GetString("dir"); dir != "" {
		return nil, fmt.Errorf("--dir and --repo name two repositories; give one")
	}
	parsed, err := remote.Parse(repo)
	if err != nil {
		return nil, err
	}
	return &Config{Repo: &parsed}, nil
}

// CreateConfig builds the scan command's Config from its flags and config file.
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
	cfg.RequiredChecks, _ = flags.GetBool("required-checks")
	cfg.Baseline, _ = flags.GetString("baseline")
	cfg.WriteBaseline, _ = flags.GetBool("write-baseline")
	cfg.Badge, _ = flags.GetString("badge")
	cfg.OutputFile, _ = flags.GetString("output-file")
	cfg.Output, _ = flags.GetString("output")
	cfg.Output = strings.ToLower(cfg.Output)

	// --write-baseline with no --baseline writes to the default location, in
	// the scanned directory; a fetched copy is removed, so then the current one.
	if cfg.WriteBaseline && cfg.Baseline == "" {
		cfg.Baseline = baseline.DefaultPath
		if cfg.Repo == nil {
			cfg.Baseline = filepath.Join(cfg.Dir, baseline.DefaultPath)
		}
	}
	if !slices.Contains(OutputFormats, cfg.Output) {
		return nil, fmt.Errorf("unsupported output format: %s (expected one of %s)", cfg.Output, strings.Join(OutputFormats, ", "))
	}
	return cfg, nil
}

// CreateSkillsConfig builds the skills command's Config. Skills are read from
// a checkout only: a fetched repository keeps CI files alone.
func CreateSkillsConfig(cmd *cobra.Command) (*Config, error) {
	dir, err := resolveDir(cmd)
	if err != nil {
		return nil, err
	}
	cfg := &Config{Dir: dir}
	cfg.Enforce, _ = cmd.Flags().GetBool("enforce")
	cfg.Output, _ = cmd.Flags().GetString("output")
	cfg.Output = strings.ToLower(cfg.Output)
	if cfg.Output != OutputTable && cfg.Output != OutputJSON {
		return nil, fmt.Errorf("unsupported output format: %s (expected table or json)", cfg.Output)
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
	if cfg.Write && cfg.Repo != nil {
		return nil, fmt.Errorf("--write adds a workflow to a checkout; use --dir, or drop --write to print the snippets")
	}
	return cfg, nil
}
