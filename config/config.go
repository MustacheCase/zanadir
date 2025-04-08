package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const (
	OutputJSON  = "json"
	OutputTable = "table"
)

type Config struct {
	Dir                string
	ExcludedCategories []string
	Enforce            bool
	Debug              bool
	Output             string
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
	enforce, _ := cmd.Flags().GetBool("enforce")
	debug, _ := cmd.Flags().GetBool("debug")
	output, _ := cmd.Flags().GetString("output")
	// Validate that output is either OutputJSON or OutputTable
	if output != OutputJSON && output != OutputTable {
		return nil, fmt.Errorf("unsupported output format: %s", output)
	}

	return &Config{
		Dir:                dir,
		ExcludedCategories: excludedCategories,
		Enforce:            enforce,
		Debug:              debug,
		Output:             output,
	}, nil
}
