package handler

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/MustacheCase/zanadir/score"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests feed zanadir configurations written to test it, and GitLab
// support crashed on most real files for months while all of them passed.
// TestCorpus scans real repositories, pinned in testdata/corpus/repos.txt, and
// compares what zanadir concludes with a committed snapshot, so a crash fails
// and a change in behaviour shows up as a diff in the pull request that made
// it.
//
//	scripts/fetch-corpus.sh handler/testdata/corpus/repos.txt .corpus
//	CORPUS_DIR=$PWD/.corpus go test ./handler -run TestCorpus [-update-corpus]
var updateCorpus = flag.Bool("update-corpus", false, "rewrite the corpus snapshots from this build")

const corpusData = "testdata/corpus"

// corpusSnapshot is what one scan concluded, in a shape whose diff a reviewer
// can read.
type corpusSnapshot struct {
	Platforms     []string          `json:"platforms"`
	Artifacts     int               `json:"artifacts"`
	Jobs          int               `json:"jobs"`
	Coverage      string            `json:"coverage"`
	Uncovered     []string          `json:"uncovered"`
	NotApplicable []string          `json:"notApplicable"`
	Verdicts      map[string]string `json:"verdicts"`
}

// corpusRepos reads the manifest into the directory name each repository is
// fetched to.
func corpusRepos(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(filepath.Join(corpusData, "repos.txt"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	var dirs []string
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		fields := strings.Fields(lines.Text())
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		require.Len(t, fields, 3, "manifest line %q", lines.Text())
		// A branch or tag would move, and the snapshot with it.
		require.Regexp(t, `^[0-9a-f]{40}$`, fields[2], "%s must be pinned to a full commit hash", fields[1])
		dirs = append(dirs, fields[0]+"_"+strings.ReplaceAll(fields[1], "/", "_"))
	}
	require.NoError(t, lines.Err())
	return dirs
}

func TestCorpus(t *testing.T) {
	root := os.Getenv("CORPUS_DIR")
	if root == "" {
		t.Skip("CORPUS_DIR is not set; see the comment on TestCorpus")
	}
	h, err := Setup()
	require.NoError(t, err)

	repos := corpusRepos(t)
	for _, name := range repos {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(root, name)
			require.DirExists(t, dir, "fetch the corpus first")

			result, err := h.analyse(&config.Config{Dir: dir}, func(string, ...interface{}) {}, nil)
			require.NoError(t, err, "a real repository must never abort the scan")
			got := snapshotOf(dir, result)

			golden := filepath.Join(corpusData, name+".json")
			if *updateCorpus {
				data, err := json.MarshalIndent(got, "", "  ")
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(golden, append(data, '\n'), 0o600))
				return
			}
			data, err := os.ReadFile(golden)
			require.NoError(t, err, "no snapshot yet: rerun with -update-corpus and review it")
			var want corpusSnapshot
			require.NoError(t, json.Unmarshal(data, &want))
			assert.Equal(t, want, got, "zanadir now concludes something different about %s; if that is intended, rerun with -update-corpus", name)
		})
	}

	// A snapshot whose repository left the manifest would never be checked.
	snapshots, err := filepath.Glob(filepath.Join(corpusData, "*.json"))
	require.NoError(t, err)
	for _, path := range snapshots {
		assert.Contains(t, repos, strings.TrimSuffix(filepath.Base(path), ".json"), "snapshot without a repository in repos.txt")
	}
}

func snapshotOf(dir string, result *scanResult) corpusSnapshot {
	snap := corpusSnapshot{
		Platforms:     []string{},
		Artifacts:     len(result.artifacts),
		Coverage:      score.Of(result.skipped, len(result.suggestions)).String(),
		Uncovered:     []string{},
		NotApplicable: append([]string{}, result.notApplicable...),
		Verdicts:      map[string]string{},
	}
	for _, p := range parser.Platforms {
		if p.Parser.Exists(dir) {
			snap.Platforms = append(snap.Platforms, p.ID)
		}
	}
	for _, a := range result.artifacts {
		snap.Jobs += len(a.Jobs)
	}
	for _, s := range result.suggestions {
		snap.Uncovered = append(snap.Uncovered, s.ID)
	}
	slices.Sort(snap.Uncovered)
	for category, a := range result.assessments {
		snap.Verdicts[category] = fmt.Sprintf("%s %s", a.Verdict, repoRelative(dir, a.Location))
	}
	return snap
}
