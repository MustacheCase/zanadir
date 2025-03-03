package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCreateConfig(t *testing.T) {
	tests := []struct {
		name               string
		dir                string
		excludedCategories []string
		expectError        bool
	}{
		{
			name:               "Valid directory",
			dir:                "/tmp/testdir",
			excludedCategories: []string{},
			expectError:        false,
		},
		{
			name:               "Empty directory flag",
			dir:                "",
			excludedCategories: []string{},
			expectError:        true,
		},
		{
			name:               "Symlink directory",
			dir:                "/tmp/symlinkdir",
			excludedCategories: []string{},
			expectError:        true,
		},
		{
			name:               "Excluded categories",
			dir:                "/tmp/testdir",
			excludedCategories: []string{"cat1", "cat2"},
			expectError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("dir", tt.dir, "directory")
			cmd.Flags().StringSlice("excluded-categories", tt.excludedCategories, "excluded categories")

			if tt.dir == "/tmp/symlinkdir" {
				_ = os.Symlink("/tmp/testdir", "/tmp/symlinkdir")
				defer os.Remove("/tmp/symlinkdir")
			} else if tt.dir != "" {
				_ = os.MkdirAll(tt.dir, os.ModePerm)
				defer os.RemoveAll(tt.dir)
			}

			config, err := CreateConfig(cmd)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, filepath.Clean(tt.dir), config.Dir)
				assert.Equal(t, tt.excludedCategories, config.ExcludedCategories)
			}
		})
	}
}
