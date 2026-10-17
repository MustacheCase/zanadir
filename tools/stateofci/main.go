// Command stateofci scans the most-starred GitHub repositories that have CI
// with zanadir and aggregates the results; see README.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// exit is os.Exit, replaced in tests.
var exit = os.Exit

func main() {
	exit(cli(os.Args[1:], os.Getenv("GITHUB_TOKEN")))
}

func cli(args []string, token string) int {
	flags := flag.NewFlagSet("stateofci", flag.ContinueOnError)
	zanadir := flags.String("zanadir", "./zanadir", "the zanadir binary to scan with")
	n := flags.Int("n", 1000, "how many repositories with CI to report on")
	out := flags.String("out", ".stateofci", "where candidates, per-repository results and the summary go")
	workers := flags.Int("workers", 6, "repositories scanned at once")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := run(*zanadir, *n, *out, *workers, token); err != nil {
		fmt.Fprintln(os.Stderr, "stateofci:", err)
		return 1
	}
	return 0
}

func run(zanadir string, n int, out string, workers int, token string) error {
	if err := os.MkdirAll(filepath.Join(out, "raw"), 0o755); err != nil {
		return err
	}
	candidates, err := loadCandidates(filepath.Join(out, "candidates.json"), n, token)
	if err != nil {
		return err
	}
	scanUntil(zanadir, candidates, n, filepath.Join(out, "raw"), workers, token)
	return writeSummary(filepath.Join(out, "raw"), candidates, n, out)
}
