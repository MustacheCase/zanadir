package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MustacheCase/zanadir/baseline"
	"github.com/MustacheCase/zanadir/models"
	"github.com/spf13/cobra"
)

const (
	OutputJSON  = "json"
	OutputTable = "table"
	OutputSARIF = "sarif"
)

type Config struct {
	Dir                string
	ExcludedCategories []string
	Enforce            bool
	// FailOn limits enforcement to specific categories; empty means all.
	FailOn []string
	// Baseline is the path to a file of already-accepted gaps.
	Baseline      string
	WriteBaseline bool
	Debug         bool
	Output        string
	// OutputFile writes the report to a path instead of stdout; empty means stdout.
	OutputFile string
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

func CreateConfig(cmd *cobra.Command) (*Config, error) {
	dir, _ := cmd.Flags().GetString("dir")
	if dir == "" {
		_ = cmd.Help()
		return nil, fmt.Errorf("error: --dir (-d) flag is required")
	}

	dir = filepath.Clean(dir)

	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("error: Symlinks are not allowed")
	}

	excludedCategories, _ := cmd.Flags().GetStringSlice("excluded-categories")
	excludedCategories, err = normalizeCategories(excludedCategories)
	if err != nil {
		return nil, err
	}

	enforce, _ := cmd.Flags().GetBool("enforce")
	failOn, _ := cmd.Flags().GetStringSlice("fail-on")
	failOn, err = normalizeCategories(failOn)
	if err != nil {
		return nil, err
	}

	baselinePath, _ := cmd.Flags().GetString("baseline")
	writeBaseline, _ := cmd.Flags().GetBool("write-baseline")

	// --baseline may be given without a value, meaning the default location.
	if writeBaseline && baselinePath == "" {
		baselinePath = baseline.DefaultPath
	}

	debug, _ := cmd.Flags().GetBool("debug")
	outputFile, _ := cmd.Flags().GetString("output-file")
	output, _ := cmd.Flags().GetString("output")
	if output != OutputJSON && output != OutputTable && output != OutputSARIF {
		return nil, fmt.Errorf("unsupported output format: %s (expected %s, %s or %s)", output, OutputTable, OutputJSON, OutputSARIF)
	}

	return &Config{
		Dir:                dir,
		ExcludedCategories: excludedCategories,
		Enforce:            enforce,
		FailOn:             failOn,
		Baseline:           baselinePath,
		WriteBaseline:      writeBaseline,
		Debug:              debug,
		Output:             output,
		OutputFile:         outputFile,
	}, nil
}
