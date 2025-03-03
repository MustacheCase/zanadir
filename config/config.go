package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type Config struct {
	Dir                string
	ExcludedCategories []string
}

func CreateConfig(cmd *cobra.Command) (*Config, error) {
	dir, _ := cmd.Flags().GetString("dir")
	if dir == "" {
		_ = cmd.Help()
		return nil, fmt.Errorf("error: --dir (-d) flag is required")
	}

	// normalize the path
	dir = filepath.Clean(dir)

	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("error: Symlinks are not allowed")
	}

	excludedCategories, _ := cmd.Flags().GetStringSlice("excluded-categories")

	return &Config{
		Dir:                dir,
		ExcludedCategories: excludedCategories,
	}, nil
}
