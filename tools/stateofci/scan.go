package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// result is the part of zanadir's JSON output the report reads.
type result struct {
	Score struct {
		Covered int `json:"covered"`
		Total   int `json:"total"`
	} `json:"score"`
	Platforms []string `json:"platforms"`
	Uncovered []struct {
		ID string `json:"id"`
	} `json:"uncovered"`
	Weakened []struct {
		Category string `json:"category"`
		Verdict  string `json:"verdict"`
		Location string `json:"location"`
	} `json:"weakened"`
	NotApplicable []string `json:"notApplicable"`
}

// perRepository bounds one scan; a stalled download must not hold a worker.
// A rate-limited download is retried after backoff, which grows each time.
var (
	perRepository = 10 * time.Minute
	backoff       = time.Minute
	attempts      = 3
)

func rawPath(dir, name string) string {
	return filepath.Join(dir, strings.ReplaceAll(name, "/", "__")+".json")
}

// readResult returns a repository's saved result, and whether one exists.
func readResult(dir, name string) (result, bool) {
	data, err := os.ReadFile(rawPath(dir, name))
	if err != nil {
		return result{}, false
	}
	var r result
	return r, json.Unmarshal(data, &r) == nil
}

// scanUntil scans candidates in star order, a batch at a time, until n of
// them have CI. A repository with a saved result or error is not scanned
// again.
func scanUntil(zanadir string, candidates []candidate, n int, dir string, workers int, token string) {
	withCI := 0
	for start := 0; start < len(candidates) && withCI < n; start += workers {
		batch := candidates[start:min(start+workers, len(candidates))]
		var wg sync.WaitGroup
		for _, c := range batch {
			if _, done := readResult(dir, c.FullName); done || exists(rawPath(dir, c.FullName)+".err") {
				continue
			}
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				scanOne(zanadir, name, dir, token)
			}(c.FullName)
		}
		wg.Wait()
		for _, c := range batch {
			if r, ok := readResult(dir, c.FullName); ok && len(r.Platforms) > 0 {
				withCI++
			}
		}
		fmt.Printf("scanned %d of %d candidates, %d with CI\n", min(start+workers, len(candidates)), len(candidates), withCI)
	}
}

// scanOne saves zanadir's JSON for a repository, or what went wrong beside it.
func scanOne(zanadir, name, dir, token string) {
	var stdout, stderr bytes.Buffer
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		stdout.Reset()
		stderr.Reset()
		err = runZanadir(zanadir, name, token, &stdout, &stderr)
		if err == nil || !strings.Contains(stdout.String()+stderr.String(), "429 Too Many Requests") {
			break
		}
		time.Sleep(time.Duration(attempt) * backoff)
	}

	var r result
	if err == nil {
		err = json.Unmarshal(stdout.Bytes(), &r)
	}
	if err != nil {
		_ = os.WriteFile(rawPath(dir, name)+".err", fmt.Appendf(nil, "%v\n%s%s", err, stdout.Bytes(), stderr.Bytes()), 0o600)
		return
	}
	_ = os.WriteFile(rawPath(dir, name), stdout.Bytes(), 0o600)
}

func runZanadir(zanadir, name, token string, stdout, stderr *bytes.Buffer) error {
	ctx, cancel := context.WithTimeout(context.Background(), perRepository)
	defer cancel()
	cmd := exec.CommandContext(ctx, zanadir, "scan", "--repo", name, "-o", "json")
	cmd.Env = append(os.Environ(), "GITHUB_TOKEN="+token)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
