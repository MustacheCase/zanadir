// Command cataloguehealth checks that the tools zanadir recommends, and the
// actions its fix templates pin, are still alive; see README.md.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

// exit is os.Exit, replaced in tests.
var exit = os.Exit

func main() {
	exit(cli(os.Args[1:], os.Getenv("GITHUB_TOKEN"), time.Now(), os.Stdout))
}

func cli(args []string, token string, now time.Time, stdout io.Writer) int {
	flags := flag.NewFlagSet("cataloguehealth", flag.ContinueOnError)
	root := flags.String("root", ".", "the zanadir checkout whose catalogue to check")
	out := flags.String("out", "", "write the report to this file - empty when the catalogue is healthy - instead of stdout")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	findings, err := check(*root, token, now)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "cataloguehealth:", err)
		return 1
	}
	report := render(findings)
	if *out == "" {
		_, _ = fmt.Fprint(stdout, report)
		return 0
	}
	if len(findings) == 0 {
		report = ""
	}
	if err := os.WriteFile(*out, []byte(report), 0o600); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "cataloguehealth:", err)
		return 1
	}
	return 0
}
