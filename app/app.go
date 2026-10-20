package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/handler"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/org"
	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/remote"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "zanadir",
	Short: "Grades whether a repository's CI actually protects it",
	Long:  "zanadir reads a repository's CI configuration and reports which security and quality controls are missing, or wired so they cannot fail a build.",
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Args:  noArgs,
	Short: "Scans a repository's CI configuration",
	Long:  "The scan command reads a repository's CI configuration and reports the categories it does not cover.",
	Run:   run(config.CreateConfig, scanRepo),
}

var fixCmd = &cobra.Command{
	Use:   "fix",
	Args:  noArgs,
	Short: "Prints CI configuration for the categories a scan reports as missing",
	Long:  "The fix command prints ready-to-paste CI configuration for each uncovered category, so a scan result can be acted on without leaving the tool.",
	Run:   run(config.CreateFixConfig, fixRepo),
}

// exit, stderr, fetchRepository and interrupts are replaced in tests.
var (
	exit                       = os.Exit
	stderr           io.Writer = os.Stderr
	fetchRepository            = remote.FetchContext
	scanOrganisation           = org.Scan
	interrupts                 = defaultInterrupts
)

func defaultInterrupts() (<-chan os.Signal, func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	return signals, func() { signal.Stop(signals) }
}

// githubToken is the token a remote scan sends, if the environment has one.
// run builds a command's configuration and does its work, exiting non-zero
// on any failure.
func run(build func(*cobra.Command) (*config.Config, error), do func(context.Context, *config.Config) error) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, _ []string) {
		cfg, err := build(cmd)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
			exit(1)
			return
		}

		// A cancelled CI job sends SIGTERM. The download is abandoned, and the
		// fetched copy removed, before exiting as the signal would have.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		signals, stop := interrupts()
		defer stop()
		var mu sync.Mutex
		cleanup := func() {}
		fetched, finished, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
		// An organisation scan stops on the signal itself, cleaning up and
		// writing what it has, so the exit waits for it.
		worked := make(chan struct{})
		defer close(finished)
		go func() {
			select {
			case sig := <-signals:
				cancel()
				<-fetched
				if cfg.Org != "" {
					<-worked
				}
				mu.Lock()
				cleanup()
				mu.Unlock()
				exit(128 + signalNumber(sig))
				close(exited)
			case <-finished:
			}
		}()
		// Once interrupted, the exit is the signal's: exit returns only in tests.
		interrupted := func() bool {
			if ctx.Err() != nil {
				<-exited
				return true
			}
			return false
		}

		if cfg.Repo != nil {
			var remove func()
			cfg.Dir, remove, err = fetchRepository(ctx, *cfg.Repo, remote.Token())
			if err == nil {
				mu.Lock()
				cleanup = remove
				mu.Unlock()
			}
		}
		close(fetched)
		if interrupted() {
			return
		}
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
			exit(1)
			return
		}
		// exit does not run deferred calls, so the fetched copy goes first.
		err = do(ctx, cfg)
		close(worked)
		if interrupted() {
			return
		}
		mu.Lock()
		cleanup()
		mu.Unlock()
		if err != nil {
			_, _ = fmt.Fprint(stderr, scanErrorReport(err))
			exit(1)
		}
	}
}

// noArgs refuses a stray word, as in --enforce false, which would otherwise
// be ignored. An empty argument is not one: the runner passes an action's
// unset optional input to the container as "".
func noArgs(cmd *cobra.Command, args []string) error {
	return cobra.NoArgs(cmd, slices.DeleteFunc(slices.Clone(args), func(arg string) bool { return arg == "" }))
}

func signalNumber(sig os.Signal) int {
	if number, ok := sig.(syscall.Signal); ok {
		return int(number)
	}
	return 1
}

// addSharedFlags defines the flags scan and fix both take.
func addSharedFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("dir", "d", "", "Path to the repository directory (required unless --repo is given)")
	cmd.Flags().String("repo", "", "Scan a GitHub repository without cloning it: github.com/owner/name[@ref]; uses GITHUB_TOKEN if set (optional)")
	cmd.Flags().StringSliceP("excluded-categories", "e", []string{}, "List of excluded categories (optional)")
	cmd.Flags().Bool("debug", false, "Run the tool using debug mode (optional)")
	cmd.Flags().StringP("config", "c", "", "Read flag values from this YAML file; flags given on the command line win (optional)")
}

// NewApp initializes the CLI application
func NewApp() *cobra.Command {
	rootCmd.AddCommand(scanCmd, fixCmd)

	addSharedFlags(scanCmd)
	scanCmd.Flags().String("org", "", "Scan every repository of a GitHub organisation: github.com/name; needs GITHUB_TOKEN (optional)")
	scanCmd.Flags().Int("parallel", 4, "With --org, how many repositories to scan at a time (optional)")
	scanCmd.Flags().Bool("include-archived", false, "With --org, also scan archived repositories (optional)")
	scanCmd.Flags().Bool("include-forks", false, "With --org, also scan forks (optional)")
	scanCmd.Flags().Bool("enforce", false, "Fails the CI process when any category is uncovered (optional)")
	scanCmd.Flags().StringSlice("fail-on", []string{}, "Fail only when these specific categories are uncovered (optional)")
	scanCmd.Flags().Bool("fail-on-weakened", false, "Also fail when a category has tooling that cannot fail a build (optional)")
	scanCmd.Flags().Bool("required-checks", false, "Read the default branch's required checks from GitHub: a control enforces only if its check must pass to merge; uses GITHUB_TOKEN if set (optional)")
	scanCmd.Flags().String("baseline", "", "Path to a baseline file of already-accepted gaps (optional)")
	scanCmd.Flags().Bool("write-baseline", false, "Write the current uncovered categories to the baseline file and exit successfully (optional)")
	scanCmd.Flags().StringP("output", "o", "table", "Output format of the tool (table, json, sarif, markdown) (optional)")
	scanCmd.Flags().String("output-file", "", "Write the report to this file instead of stdout (optional)")
	scanCmd.Flags().String("badge", "", "Write a shields.io endpoint badge of the coverage score to this path (optional)")

	addSharedFlags(fixCmd)
	fixCmd.Flags().Bool("write", false, "Generate .github/workflows/zanadir-suggested.yml instead of printing (optional)")

	return rootCmd
}

// scanErrorReport words a failed scan. An enforcement failure is the tool
// doing its job, so it names the categories - not obvious once --fail-on or a
// baseline narrows enforcement down. Both go to stderr, so a JSON or SARIF
// report on stdout stays parseable.
func scanErrorReport(err error) string {
	var enforceErr *models.EnforceError
	if errors.As(err, &enforceErr) {
		return fmt.Sprintf("Enforcement failed: %v\n", err)
	}
	return fmt.Sprintf("Error: scan repo failed: %v\n", err)
}

func scanRepo(ctx context.Context, cfg *config.Config) error {
	h, err := handler.Setup(cfg.Rules...)
	if err != nil {
		return err
	}
	if cfg.Org != "" {
		return scanOrg(ctx, h, cfg)
	}
	return h.Execute(cfg)
}

// scanOrg scans every repository of cfg.Org, each graded as a --repo scan.
func scanOrg(ctx context.Context, h *handler.Handler, cfg *config.Config) error {
	token := remote.Token()
	if token == "" {
		return errors.New("--org needs GITHUB_TOKEN: without one GitHub allows about 20 repositories an hour")
	}
	report, err := scanOrganisation(ctx, org.Options{
		Org: cfg.Org, Token: token, Parallel: cfg.Parallel,
		IncludeArchived: cfg.IncludeArchived, IncludeForks: cfg.IncludeForks, Progress: stderr,
		Assess: func(repo remote.Repository, dir string) (output.Report, error) {
			repoCfg := *cfg
			repoCfg.Org, repoCfg.Repo, repoCfg.Dir = "", &repo, dir
			return h.Assess(&repoCfg)
		},
	})
	if err != nil {
		return err
	}
	report.Format, report.DestPath = cfg.Output, cfg.OutputFile
	if err := output.WriteOrg(report); err != nil {
		return err
	}
	if len(report.Repositories) == 0 && len(report.Failed) > 0 {
		return fmt.Errorf("none of the %d repositories of %s could be scanned", len(report.Failed), cfg.Org)
	}
	return nil
}

func fixRepo(_ context.Context, cfg *config.Config) error {
	h, err := handler.Setup(cfg.Rules...)
	if err != nil {
		return err
	}
	return h.Fix(cfg, os.Stdout)
}
