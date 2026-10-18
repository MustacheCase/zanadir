package main

import (
	"os"

	"github.com/MustacheCase/zanadir/app"
	"github.com/spf13/cobra"
)

// exit is os.Exit, replaced in tests.
var exit = os.Exit

func main() {
	exit(run(app.NewApp(), os.Args[1:]))
}

// run executes the command line. A scan or fix that fails exits 1 from within
// the command, so an error reaching here is one cobra raised while parsing -
// an unknown flag or command - and has already been printed with the usage.
func run(cmd *cobra.Command, args []string) int {
	cmd.SetArgs(args)
	if cmd.Execute() != nil {
		return 2
	}
	return 0
}
