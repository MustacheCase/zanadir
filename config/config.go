package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	Debug              bool
	Output             string
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
	debug, _ := cmd.Flags().GetBool("debug")
	output, _ := cmd.Flags().GetString("output")
	if output != OutputJSON && output != OutputTable && output != OutputSARIF {
		return nil, fmt.Errorf("unsupported output format: %s (expected %s, %s or %s)", output, OutputTable, OutputJSON, OutputSARIF)
	}

	return &Config{
		Dir:                dir,
		ExcludedCategories: excludedCategories,
		Enforce:            enforce,
		Debug:              debug,
		Output:             output,
	}, nil
}
