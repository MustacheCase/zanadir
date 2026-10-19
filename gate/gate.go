// Package gate reads what GitHub requires before a pull request can merge
// into a repository's default branch: the status checks that rulesets and
// branch protection make required.
package gate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/remote"
)

// actionsApp is the GitHub App that reports GitHub Actions jobs as checks.
const actionsApp = 15368

// Check is a required status check and the app expected to report it; 0 when
// any app may.
type Check struct {
	Context string
	AppID   int
}

// Gate is what a merge into Branch waits for: required checks, and code
// scanning tools whose alerts block it.
type Gate struct {
	Branch   string
	Checks   []Check
	Scanning []string
	Problems []string
}

// Blocks reports whether a job holds back a merge: its check is required, or
// it runs a tool whose code scanning alerts are.
func (g *Gate) Blocks(job *models.Job) bool {
	if g.Requires(job) {
		return true
	}
	text := strings.ToLower(job.Package + " " + job.Run)
	return slices.ContainsFunc(g.Scanning, func(tool string) bool {
		name, _, _ := strings.Cut(strings.ToLower(tool), " ")
		return name != "" && strings.Contains(text, name)
	})
}

// Requires reports whether a job's check must pass to merge.
func (g *Gate) Requires(job *models.Job) bool {
	return slices.ContainsFunc(g.Checks, func(c Check) bool { return produces(job, c) })
}

var expression = regexp.MustCompile(`\$\{\{.*?\}\}`)

// produces reports whether a GitHub Actions job reports a required check: one
// bound to no app or to Actions. A matrix job reports as "name (values)", a
// job calling a reusable workflow as "name / called job", and an expression
// in the name may evaluate to anything - though a name that is nothing but
// expressions says nothing about which check it is.
func produces(job *models.Job, check Check) bool {
	if check.AppID != 0 && check.AppID != actionsApp || job.UnnamedCheck() {
		return false
	}
	parts := expression.Split(job.Check, -1)
	for i, part := range parts {
		parts[i] = regexp.QuoteMeta(part)
	}
	suffix := `(?: \(.*\))?`
	if job.CallsWorkflow() {
		suffix = `(?: \(.*\)| / .*)?`
	}
	return regexp.MustCompile("^" + strings.Join(parts, ".*") + suffix + "$").MatchString(check.Context)
}

// Unproduced adds a problem for each required check that GitHub Actions is
// expected to report but no job does: a pull request then waits for it
// forever. Checks other apps report, such as DCO, are not judged, and a job
// named only by expressions may be the one that reports it.
func (g *Gate) Unproduced(jobs []*models.Job) {
	if slices.ContainsFunc(jobs, (*models.Job).UnnamedCheck) {
		return
	}
	for _, check := range g.Checks {
		if check.AppID != actionsApp {
			continue
		}
		if !slices.ContainsFunc(jobs, func(job *models.Job) bool { return produces(job, check) }) {
			g.Problems = append(g.Problems, fmt.Sprintf("required check %q is reported by no workflow job, so pull requests wait for it", check.Context))
		}
	}
}

// apiBase and client are replaced in tests.
var (
	apiBase = "https://api.github.com"
	client  = &http.Client{Timeout: time.Minute}
)

var errNotFound = errors.New("not found")

// Read fetches the gate on repo's default branch. Public repositories need no
// token.
func Read(repo remote.Repository, token string) (*Gate, error) {
	base := "/repos/" + repo.Owner + "/" + repo.Name
	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := get(base, token, &info); err != nil {
		return nil, fmt.Errorf("reading %s: %w", repo, err)
	}
	g := &Gate{Branch: info.DefaultBranch}
	branch := url.PathEscape(info.DefaultBranch)

	rules, err := getAll[rule](base+"/rules/branches/"+branch, token)
	if err != nil {
		return nil, fmt.Errorf("reading the rules on %s: %w", g.Branch, err)
	}
	for _, r := range rules {
		g.Checks = append(g.Checks, r.checks()...)
		for _, tool := range r.Parameters.CodeScanningTools {
			// Both thresholds at none: the tool's alerts block nothing.
			if tool.AlertsThreshold != "none" || tool.SecurityAlertsThreshold != "none" {
				g.Scanning = append(g.Scanning, tool.Tool)
			}
		}
	}

	var protected struct {
		Protection struct {
			RequiredStatusChecks struct {
				EnforcementLevel string `json:"enforcement_level"`
				Checks           []struct {
					Context string `json:"context"`
					AppID   *int   `json:"app_id"`
				} `json:"checks"`
			} `json:"required_status_checks"`
		} `json:"protection"`
	}
	if err := get(base+"/branches/"+branch, token, &protected); err != nil {
		return nil, fmt.Errorf("reading the protection of %s: %w", g.Branch, err)
	}
	if required := protected.Protection.RequiredStatusChecks; required.EnforcementLevel != "off" {
		for _, c := range required.Checks {
			g.Checks = append(g.Checks, Check{Context: c.Context, AppID: deref(c.AppID)})
		}
	}

	g.Problems = idleRulesets(base, token)
	return g, nil
}

type rule struct {
	Type       string `json:"type"`
	Parameters struct {
		RequiredStatusChecks []struct {
			Context       string `json:"context"`
			IntegrationID *int   `json:"integration_id"`
		} `json:"required_status_checks"`
		CodeScanningTools []struct {
			Tool                    string `json:"tool"`
			AlertsThreshold         string `json:"alerts_threshold"`
			SecurityAlertsThreshold string `json:"security_alerts_threshold"`
		} `json:"code_scanning_tools"`
	} `json:"parameters"`
}

func (r rule) checks() []Check {
	if r.Type != "required_status_checks" {
		return nil
	}
	var checks []Check
	for _, c := range r.Parameters.RequiredStatusChecks {
		checks = append(checks, Check{Context: c.Context, AppID: deref(c.IntegrationID)})
	}
	return checks
}

// idleRulesets names rulesets that require checks yet hold back no merge:
// one only evaluating, or one targeting no branch. Reading a ruleset's
// details can be refused, so this is best effort.
func idleRulesets(base, token string) []string {
	listed, err := getAll[struct {
		ID int `json:"id"`
	}](base+"/rulesets", token)
	if err != nil {
		return nil
	}
	var problems []string
	for _, l := range listed {
		var rs struct {
			Name        string `json:"name"`
			Target      string `json:"target"`
			Enforcement string `json:"enforcement"`
			Conditions  struct {
				RefName struct {
					Include []string `json:"include"`
				} `json:"ref_name"`
			} `json:"conditions"`
			Rules []rule `json:"rules"`
		}
		if get(fmt.Sprintf("%s/rulesets/%d", base, l.ID), token, &rs) != nil || rs.Target != "branch" {
			continue
		}
		var contexts []string
		for _, r := range rs.Rules {
			for _, c := range r.checks() {
				contexts = append(contexts, fmt.Sprintf("%q", c.Context))
			}
		}
		switch {
		case len(contexts) == 0:
		case rs.Enforcement != "active":
			problems = append(problems, fmt.Sprintf("ruleset %q requires %s but is %s, so it blocks nothing", rs.Name, strings.Join(contexts, ", "), rs.Enforcement))
		case len(rs.Conditions.RefName.Include) == 0:
			problems = append(problems, fmt.Sprintf("ruleset %q requires %s but applies to no branch", rs.Name, strings.Join(contexts, ", ")))
		}
	}
	return problems
}

// getAll reads every page of a list, a hundred at a time: a repository with
// more than the default thirty rules would otherwise lose some.
func getAll[T any](path, token string) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		var items []T
		if err := get(fmt.Sprintf("%s?per_page=100&page=%d", path, page), token, &items); err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < 100 {
			return all, nil
		}
	}
}

func get(path, token string, out any) error {
	req, err := http.NewRequest(http.MethodGet, apiBase+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "zanadir")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return errNotFound
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("GitHub answered %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func deref(id *int) int {
	if id == nil {
		return 0
	}
	return *id
}

// DefaultBranch reads the branch a checkout's origin/HEAD names, which git
// clone sets; "" when it is not set, as in most CI checkouts.
func DefaultBranch(dir string) string {
	head, err := os.ReadFile(filepath.Join(commonDir(dir), "refs", "remotes", "origin", "HEAD"))
	if err != nil {
		return ""
	}
	branch, found := strings.CutPrefix(strings.TrimSpace(string(head)), "ref: refs/remotes/origin/")
	if !found {
		return ""
	}
	return branch
}

var originURL = regexp.MustCompile(`(?m)^\s*url\s*=\s*(\S+)\s*$`)

// commonDir is the git directory holding a checkout's remotes. A worktree or
// a submodule has a .git file naming its git directory, and a worktree's
// names, in commondir, the one it shares. git writes these, never a
// repository's content, so they may lead outside the checkout.
func commonDir(dir string) string {
	git := filepath.Join(dir, ".git")
	if link, err := os.ReadFile(git); err == nil {
		if target, ok := strings.CutPrefix(strings.TrimSpace(string(link)), "gitdir:"); ok {
			git = relativeTo(dir, strings.TrimSpace(target))
		}
	}
	if common, err := os.ReadFile(filepath.Join(git, "commondir")); err == nil {
		return relativeTo(git, strings.TrimSpace(string(common)))
	}
	return git
}

func relativeTo(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

// Origin reads the GitHub repository a checkout's origin remote points at.
func Origin(dir string) (remote.Repository, error) {
	config, err := os.ReadFile(filepath.Join(commonDir(dir), "config"))
	if err != nil {
		return remote.Repository{}, err
	}
	_, section, found := strings.Cut(string(config), `[remote "origin"]`)
	if !found {
		return remote.Repository{}, errors.New("the checkout has no origin remote")
	}
	section, _, _ = strings.Cut(section, "\n[")
	match := originURL.FindStringSubmatch(section)
	if match == nil {
		return remote.Repository{}, errors.New("the origin remote has no url")
	}
	// git@github.com:owner/name.git is the SSH form of the same address.
	address := strings.TrimPrefix(strings.TrimPrefix(match[1], "ssh://"), "git@")
	return remote.Parse(strings.Replace(address, "github.com:", "github.com/", 1))
}
