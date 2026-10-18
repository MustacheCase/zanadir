package main

import (
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func command() *cobra.Command {
	cmd := &cobra.Command{Use: "zanadir"}
	scan := &cobra.Command{Use: "scan", Run: func(*cobra.Command, []string) {}}
	scan.Flags().Bool("enforce", false, "")
	cmd.AddCommand(scan)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
}

// A typo in a CI invocation must fail the build rather than pass it with
// nothing enforced.
func TestRunFailsOnUsageErrors(t *testing.T) {
	assert.Equal(t, 0, run(command(), []string{"scan", "--enforce"}))
	assert.Equal(t, 2, run(command(), []string{"scan", "--enforec"}))
	assert.Equal(t, 2, run(command(), []string{"scna"}))
	assert.Equal(t, 2, run(command(), []string{"scan", "--enforce=maybe"}))
}

func TestMainExitsWithRunsCode(t *testing.T) {
	var code int
	exit = func(c int) { code = c }
	args := os.Args
	os.Args = []string{"zanadir", "no-such-command"}
	t.Cleanup(func() { exit, os.Args = os.Exit, args })

	main()
	assert.Equal(t, 2, code)
}
