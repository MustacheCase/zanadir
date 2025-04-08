package output

import (
	"testing"

	"github.com/MustacheCase/zanadir/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestOutputFlag(t *testing.T) {
	t.Run("supported output", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("output", "json", "output format")
		// Use a temporary directory for testing.
		cmd.Flags().String("dir", t.TempDir(), "directory")
		cmd.Flags().StringSlice("excluded-categories", []string{}, "excluded categories")
		cfg, err := config.CreateConfig(cmd)
		assert.NoError(t, err)
		assert.NotNil(t, cfg)
	})

	t.Run("unsupported output", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("output", "xml", "output format")
		cmd.Flags().String("dir", t.TempDir(), "directory")
		cmd.Flags().StringSlice("excluded-categories", []string{}, "excluded categories")
		cfg, err := config.CreateConfig(cmd)
		assert.Error(t, err)
		assert.Nil(t, cfg)
	})
}
