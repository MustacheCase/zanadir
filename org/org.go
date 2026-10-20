// Package org scans every repository of a GitHub organisation, through the
// remote scan, and gathers their reports into one.
package org

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/remote"
)

// Options say what to scan and how.
type Options struct {
	Org             string
	Token           string
	Parallel        int
	IncludeArchived bool
	IncludeForks    bool
	// Progress gets a line per repository as it is done.
	Progress io.Writer
	// Assess scans one fetched repository.
	Assess func(repo remote.Repository, dir string) (output.Report, error)
}

// list, fetch and sleep are replaced in tests.
var (
	list  = remote.ListRepositories
	fetch = remote.FetchContext
	sleep = func(ctx context.Context, d time.Duration) error {
		select {
		case <-time.After(d):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// maxRateLimitWait is the longest a scan waits for GitHub's rate limit
	// to reset before giving the repository up.
	maxRateLimitWait = 15 * time.Minute
)

// Scan lists the organisation's repositories and scans those it keeps, at
// most Parallel at a time. A repository that cannot be scanned is reported
// with its error and the rest go on. Cancelling ctx stops new scans, and the
// report covers those finished.
func Scan(ctx context.Context, opts Options) (output.OrgReport, error) {
	report := output.OrgReport{Org: opts.Org}
	listed, err := list(ctx, opts.Org, opts.Token)
	if err != nil {
		return report, err
	}
	slices.SortFunc(listed, func(a, b remote.Listed) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })

	var targets []string
	for _, r := range listed {
		name := opts.Org + "/" + r.Name
		switch {
		case r.Archived && !opts.IncludeArchived:
			report.Skipped = append(report.Skipped, output.OrgRepository{Name: name, Reason: "archived"})
		case r.Fork && !opts.IncludeForks:
			report.Skipped = append(report.Skipped, output.OrgRepository{Name: name, Reason: "a fork"})
		case r.Size == 0:
			report.Skipped = append(report.Skipped, output.OrgRepository{Name: name, Reason: "empty"})
		default:
			targets = append(targets, r.Name)
		}
	}

	results := make([]*output.OrgRepository, len(targets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for range max(opts.Parallel, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				result := scanOne(ctx, opts, remote.Repository{Owner: opts.Org, Name: targets[i]})
				if ctx.Err() != nil && result.Reason != "" {
					continue
				}
				results[i] = &result
				mu.Lock()
				done++
				progress(opts.Progress, done, len(targets), result)
				mu.Unlock()
			}
		}()
	}
feed:
	for i := range targets {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

	report.Interrupted = ctx.Err() != nil
	for _, result := range results {
		switch {
		case result == nil:
		case result.Reason != "":
			report.Failed = append(report.Failed, *result)
		default:
			report.Repositories = append(report.Repositories, *result)
		}
	}
	return report, nil
}

// scanOne fetches, assesses and removes one repository, waiting out a rate
// limit that resets soon enough.
func scanOne(ctx context.Context, opts Options, repo remote.Repository) output.OrgRepository {
	name := repo.Owner + "/" + repo.Name
	for {
		dir, cleanup, err := fetch(ctx, repo, opts.Token)
		var limit *remote.RateLimitError
		if errors.As(err, &limit) {
			wait := time.Until(limit.Reset)
			if wait > maxRateLimitWait {
				return output.OrgRepository{Name: name, Reason: err.Error()}
			}
			if sleep(ctx, wait) != nil {
				return output.OrgRepository{Name: name, Reason: ctx.Err().Error()}
			}
			continue
		}
		if err != nil {
			return output.OrgRepository{Name: name, Reason: err.Error()}
		}
		report, err := opts.Assess(repo, dir)
		cleanup()
		if err != nil {
			return output.OrgRepository{Name: name, Reason: err.Error()}
		}
		return output.OrgRepository{Name: name, Report: report}
	}
}

func progress(w io.Writer, done, total int, r output.OrgRepository) {
	if w == nil {
		return
	}
	status := fmt.Sprintf("%s, %d weakened", r.Report.Score, len(r.Report.Weakened))
	if r.Reason != "" {
		status = "could not be scanned: " + r.Reason
	}
	_, _ = fmt.Fprintf(w, "[%d/%d] %s: %s\n", done, total, r.Name, status)
}
