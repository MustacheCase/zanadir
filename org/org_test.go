package org

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/remote"
	"github.com/MustacheCase/zanadir/score"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fake replaces listing and fetching for one test. fetchErrors fail a
// repository's fetch, once per error listed.
func fake(t *testing.T, listed []remote.Listed, fetchErrors map[string][]error) *[]string {
	t.Helper()
	var mu sync.Mutex
	var removed []string
	oldList, oldFetch, oldSleep := list, fetch, sleep
	t.Cleanup(func() { list, fetch, sleep = oldList, oldFetch, oldSleep })
	list = func(_ context.Context, org, token string) ([]remote.Listed, error) {
		if org == "missing" {
			return nil, errors.New("listing the repositories of missing: GitHub answered 404")
		}
		return listed, nil
	}
	fetch = func(ctx context.Context, repo remote.Repository, _ string) (string, func(), error) {
		mu.Lock()
		defer mu.Unlock()
		if errs := fetchErrors[repo.Name]; len(errs) > 0 {
			fetchErrors[repo.Name] = errs[1:]
			return "", nil, errs[0]
		}
		return "/tmp/" + repo.Name, func() {
			mu.Lock()
			removed = append(removed, repo.Name)
			mu.Unlock()
		}, nil
	}
	sleep = func(context.Context, time.Duration) error { return nil }
	return &removed
}

func assess(repo remote.Repository, dir string) (output.Report, error) {
	if repo.Name == "broken" {
		return output.Report{}, errors.New("cannot read it")
	}
	return output.Report{Score: score.Score{Covered: len(repo.Name), Total: 12}}, nil
}

func names(repositories []output.OrgRepository) []string {
	var out []string
	for _, r := range repositories {
		out = append(out, r.Name)
	}
	return out
}

func TestScanKeepsWhatItShouldAndScansIt(t *testing.T) {
	removed := fake(t, []remote.Listed{
		{Name: "web", Size: 10}, {Name: "Api", Size: 10}, {Name: "old", Archived: true, Size: 10},
		{Name: "copy", Fork: true, Size: 10}, {Name: "blank"}, {Name: "broken", Size: 10}, {Name: "gone", Size: 10},
	}, map[string][]error{"gone": {errors.New("github.com/acme/gone was not found")}})
	var progress bytes.Buffer

	report, err := Scan(context.Background(), Options{Org: "acme", Parallel: 3, Progress: &progress, Assess: assess})
	require.NoError(t, err)

	assert.Equal(t, []string{"acme/Api", "acme/web"}, names(report.Repositories), "in name order, ignoring case")
	assert.Equal(t, []string{"acme/blank", "acme/copy", "acme/old"}, names(report.Skipped))
	assert.Equal(t, []string{"empty", "a fork", "archived"}, []string{report.Skipped[0].Reason, report.Skipped[1].Reason, report.Skipped[2].Reason})
	assert.Equal(t, []string{"acme/broken", "acme/gone"}, names(report.Failed))
	assert.Equal(t, "cannot read it", report.Failed[0].Reason)
	assert.False(t, report.Interrupted)
	assert.ElementsMatch(t, []string{"Api", "web", "broken"}, *removed, "every fetched copy is removed, scanned or not")
	assert.Contains(t, progress.String(), "] acme/web: 3/12, 0 weakened\n")
	assert.Contains(t, progress.String(), "] acme/gone: could not be scanned: github.com/acme/gone was not found\n")
	assert.Contains(t, progress.String(), "[4/4]")
}

func TestScanIncludesArchivedAndForksWhenAsked(t *testing.T) {
	fake(t, []remote.Listed{{Name: "old", Archived: true, Size: 1}, {Name: "copy", Fork: true, Size: 1}}, nil)
	report, err := Scan(context.Background(), Options{Org: "acme", IncludeArchived: true, IncludeForks: true, Assess: assess})
	require.NoError(t, err)
	assert.Equal(t, []string{"acme/copy", "acme/old"}, names(report.Repositories))
}

func TestScanWaitsOutARateLimitThatResetsSoon(t *testing.T) {
	soon := &remote.RateLimitError{Reset: time.Now().Add(time.Minute)}
	late := &remote.RateLimitError{Reset: time.Now().Add(time.Hour)}
	fake(t, []remote.Listed{{Name: "a", Size: 1}, {Name: "b", Size: 1}}, map[string][]error{"a": {soon, soon}, "b": {late}})
	var waited []time.Duration
	sleep = func(_ context.Context, d time.Duration) error { waited = append(waited, d); return nil }

	report, err := Scan(context.Background(), Options{Org: "acme", Parallel: 1, Assess: assess})
	require.NoError(t, err)
	assert.Equal(t, []string{"acme/a"}, names(report.Repositories), "retried after each wait")
	assert.Len(t, waited, 2)
	assert.Equal(t, []string{"acme/b"}, names(report.Failed), "a limit that resets too late gives the repository up")
	assert.Contains(t, report.Failed[0].Reason, "rate limit")
}

func TestScanStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fake(t, []remote.Listed{{Name: "a", Size: 1}, {Name: "b", Size: 1}, {Name: "c", Size: 1}, {Name: "d", Size: 1}},
		map[string][]error{"b": {&remote.RateLimitError{Reset: time.Now().Add(time.Minute)}}})
	sleep = func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() }

	report, err := Scan(ctx, Options{Org: "acme", Parallel: 1, Assess: assess})
	require.NoError(t, err)
	assert.True(t, report.Interrupted)
	assert.Equal(t, []string{"acme/a"}, names(report.Repositories))
	assert.Empty(t, report.Failed, "what the cancellation stopped is not reported as a failure")
}

func TestScanReportsAListThatFails(t *testing.T) {
	fake(t, nil, nil)
	_, err := Scan(context.Background(), Options{Org: "missing", Assess: assess})
	assert.ErrorContains(t, err, "404")
}

func TestSleepReturnsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, sleep(ctx, time.Hour), context.Canceled)
	assert.NoError(t, sleep(context.Background(), time.Millisecond))
}
