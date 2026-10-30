package matcher_test

import (
	"regexp"
	"testing"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatch(t *testing.T) {
	svc := matcher.NewMatchService()

	artifacts := []*models.Artifact{
		{
			Name:     "artifact1",
			Location: "path/to/artifact1",
			Jobs: []*models.Job{
				{Package: "package1"},
			},
		},
		{
			Name:     "artifact2",
			Location: "path/to/artifact2",
			Jobs: []*models.Job{
				{Package: "package2"},
			},
		},
	}

	ruleSet := []*rules.Rule{
		{
			ID:         "rule1",
			Regex:      regexp.MustCompile("artifact1"),
			ApplyOn:    []string{"Artifact.Name"},
			Categories: []string{"CategoryA"},
		},
		{
			ID:         "rule2",
			Regex:      regexp.MustCompile("package2"),
			ApplyOn:    []string{"Job.Package"},
			Categories: []string{"CategoryB"},
		},
	}

	findings := svc.Match(artifacts, ruleSet)

	expectedFindings := []*matcher.Finding{
		{
			Category: "CategoryA",
			RuleID:   "rule1",
			Location: "path/to/artifact1",
			Artifact: artifacts[0],
			// A workflow-name match belongs to no step.
			Job:  nil,
			Rule: ruleSet[0],
		},
		{
			Category: "CategoryB",
			RuleID:   "rule2",
			Location: "path/to/artifact2",
			Artifact: artifacts[1],
			Job:      artifacts[1].Jobs[0],
			Rule:     ruleSet[1],
		},
	}

	assert.Equal(t, expectedFindings, findings)
}

func TestMatch_NoMatches(t *testing.T) {
	svc := matcher.NewMatchService()

	artifacts := []*models.Artifact{
		{
			Name:     "artifact3",
			Location: "path/to/artifact3",
			Jobs: []*models.Job{
				{Package: "package3"},
			},
		},
	}

	ruleSet := []*rules.Rule{
		{
			ID:         "rule3",
			Regex:      regexp.MustCompile("artifactX"),
			ApplyOn:    []string{"Artifact.Name"},
			Categories: []string{"CategoryX"},
		},
	}

	findings := svc.Match(artifacts, ruleSet)

	assert.Empty(t, findings)
}

// Tools invoked from a shell step were invisible before Job.Run existed.
func TestMatchRunCommands(t *testing.T) {
	m := matcher.NewMatchService()
	rule := &rules.Rule{
		ID:         "trivy-rule",
		ApplyOn:    []string{rules.FieldJobRun},
		Categories: []string{"SCA"},
		Regex:      regexp.MustCompile("(?i)trivy"),
	}

	artifacts := []*models.Artifact{{
		Name:     "ci",
		Location: ".github/workflows/ci.yml",
		Jobs:     []*models.Job{{Name: "security", Run: "trivy fs --exit-code 1 ."}},
	}}

	findings := m.Match(artifacts, []*rules.Rule{rule})
	assert.Len(t, findings, 1)
	assert.Equal(t, "SCA", findings[0].Category)
}

// unit-tests.yaml declared Job.Name but matchesRule had no case for it.
func TestMatchJobName(t *testing.T) {
	m := matcher.NewMatchService()
	rule := &rules.Rule{
		ID:         "unit-test-rule",
		ApplyOn:    []string{rules.FieldJobName},
		Categories: []string{"Unit Tests"},
		Regex:      regexp.MustCompile("(?i).*test.*"),
	}

	artifacts := []*models.Artifact{{
		Name: "pipeline",
		Jobs: []*models.Job{{Name: "run-tests"}},
	}}

	findings := m.Match(artifacts, []*rules.Rule{rule})
	assert.Len(t, findings, 1)
	assert.Equal(t, "Unit Tests", findings[0].Category)
}

func TestMatchEmptyRunIsNotMatched(t *testing.T) {
	m := matcher.NewMatchService()
	rule := &rules.Rule{
		ID:         "match-anything",
		ApplyOn:    []string{rules.FieldJobRun},
		Categories: []string{"SCA"},
		Regex:      regexp.MustCompile("(?i).*"),
	}

	artifacts := []*models.Artifact{{
		Name: "ci",
		Jobs: []*models.Job{{Name: "build", Package: "actions/checkout"}},
	}}

	assert.Empty(t, m.Match(artifacts, []*rules.Rule{rule}))
}

// The same tool can be wired up properly in one job and unable to fail the
// build in another. Reporting only the first match would decide a category's
// verdict on whichever step happened to be parsed first.
func TestMatchReportsEveryMatchingStep(t *testing.T) {
	artifact := &models.Artifact{
		Name:     "ci",
		Location: ".github/workflows/ci.yml",
		Jobs: []*models.Job{
			{Name: "scan", Package: "gitleaks/gitleaks-action", ContinueOnError: true},
			{Name: "unrelated", Package: "actions/checkout"},
			{Name: "gate", Package: "gitleaks/gitleaks-action"},
		},
	}
	rule := &rules.Rule{
		ID:         "gitleaks-rule",
		ApplyOn:    []string{rules.FieldJobPackage},
		Categories: []string{"Secrets Detection"},
		Regex:      regexp.MustCompile(`(?i)\bgitleaks\b`),
	}

	findings := matcher.NewMatchService().Match([]*models.Artifact{artifact}, []*rules.Rule{rule})

	assert.Len(t, findings, 2)
	assert.Same(t, artifact.Jobs[0], findings[0].Job)
	assert.Same(t, artifact.Jobs[2], findings[1].Job)
}

// A rule that can match the empty string would otherwise match every step
// that lacks the field it applies to.
func TestMatchIgnoresEmptyFields(t *testing.T) {
	artifact := &models.Artifact{
		Name:     "ci",
		Location: ".github/workflows/ci.yml",
		Jobs:     []*models.Job{{Name: "a"}, {Name: "b", Package: "actions/checkout"}},
	}
	rule := &rules.Rule{
		ID:         "greedy",
		ApplyOn:    []string{rules.FieldJobPackage},
		Categories: []string{"SCA"},
		Regex:      regexp.MustCompile(`^actions/checkout$|^$`),
	}

	findings := matcher.NewMatchService().Match([]*models.Artifact{artifact}, []*rules.Rule{rule})

	assert.Len(t, findings, 1)
	assert.Same(t, artifact.Jobs[1], findings[0].Job)
}

func TestRunMatchesOnlyCommandsThatRunTheTool(t *testing.T) {
	grype := &rules.Rule{ID: "grype-rule", Regex: regexp.MustCompile(`(?i)\bgrype\b`),
		ApplyOn: []string{rules.FieldJobRun}, Categories: []string{"SCA"}}

	for run, want := range map[string]bool{
		"chmod -R +x snapshot/**/grype":           false,
		"sudo cp ./grype /usr/local/bin/":         false,
		"echo 'scanning with grype'":              false,
		"mv \\\n  dist/grype bin/":                false,
		"chmod +x grype && ./grype dir:.":         true,
		"cat sbom.json | grype --fail-on high":    true,
		"GRYPE_DB_AUTO_UPDATE=false grype dir:.":  true,
		"set -e\nls bin; bin/grype --fail-on low": true,
	} {
		artifacts := []*models.Artifact{{Jobs: []*models.Job{{Run: run}}}}
		found := matcher.NewMatchService().Match(artifacts, []*rules.Rule{grype})
		assert.Equal(t, want, len(found) == 1, run)
	}
}

func TestUnlessExceptsOnlyTheCommandItMatches(t *testing.T) {
	trivy := &rules.Rule{ID: "trivy-rule", Regex: regexp.MustCompile(`\btrivy fs\b`), Unless: regexp.MustCompile(`--scanners secret`),
		ApplyOn: []string{rules.FieldJobRun}, Categories: []string{"SCA"}}

	for run, want := range map[string]bool{
		"trivy fs .":                                 true,
		"trivy fs --scanners secret .":               false,
		"trivy fs --scanners secret .\ntrivy fs .":   true,
		"trivy fs --scanners secret . && echo trivy": false,
	} {
		artifacts := []*models.Artifact{{Jobs: []*models.Job{{Run: run}}}}
		found := matcher.NewMatchService().Match(artifacts, []*rules.Rule{trivy})
		assert.Equal(t, want, len(found) == 1, run)
	}
}

func TestAToolsOwnActionOverridesTheWorkflowName(t *testing.T) {
	named := &rules.Rule{ID: "codeql-name-rule", Regex: regexp.MustCompile(`(?i)\bcodeql\b`), UnlessPackage: regexp.MustCompile(`^github/codeql-action/`),
		ApplyOn: []string{rules.FieldArtifactName}, Categories: []string{"SAST"}}
	withAction := &models.Artifact{Name: "CodeQL", Jobs: []*models.Job{{Package: "github/codeql-action/init"}}}
	elsewhere := &models.Artifact{Name: "CodeQL", Jobs: []*models.Job{{Package: "org/.github/.github/workflows/codeql.yml"}}}
	assert.Empty(t, matcher.NewMatchService().Match([]*models.Artifact{withAction}, []*rules.Rule{named}))
	assert.Len(t, matcher.NewMatchService().Match([]*models.Artifact{elsewhere}, []*rules.Rule{named}), 1, "the name still counts when the steps show nothing")
}

func TestMatchOnJobName(t *testing.T) {
	rule := &rules.Rule{ID: "unit", Regex: regexp.MustCompile(`(?i)\btests?\b`),
		ApplyOn: []string{rules.FieldJobName, "Job.Unknown"}, Categories: []string{"Unit Tests"}}
	artifacts := []*models.Artifact{{Jobs: []*models.Job{{Name: "unit tests"}, {Name: "lint"}}}}

	found := matcher.NewMatchService().Match(artifacts, []*rules.Rule{rule})
	assert.Len(t, found, 1)
	assert.Equal(t, "unit tests", found[0].Job.Name)
}

// A rule is not run on text holding none of the literals its matches need,
// so a literal it does not list is never matched, which shows it is skipped.
func TestRulesSkipTextWithoutTheirLiterals(t *testing.T) {
	rule := &rules.Rule{ID: "x", Regex: regexp.MustCompile(`(?i)\bscan\b`), Literals: []string{"gitleaks"},
		ApplyOn: []string{rules.FieldJobRun, rules.FieldJobName}, Categories: []string{"SAST"}}
	artifacts := []*models.Artifact{{Jobs: []*models.Job{{Name: "scan", Run: "scan ."}, {Name: "GitLeaks scan", Run: "./GitLeaks/scan ."}}}}

	found := matcher.NewMatchService().Match(artifacts, []*rules.Rule{rule})
	require.Len(t, found, 2)
	assert.Equal(t, "GitLeaks scan", found[0].Job.Name)
	assert.Equal(t, "GitLeaks scan", found[1].Job.Name, "literals are compared case-insensitively")
}

func TestCommandsAreTheOnesTheRuleMatches(t *testing.T) {
	gitleaks := &rules.Rule{ID: "gitleaks", Regex: regexp.MustCompile(`\bgitleaks\b`), Unless: regexp.MustCompile(`--version`)}

	assert.Equal(t, []string{"gitleaks detect --exit-code 1", "gitleaks protect"},
		matcher.Commands("trivy fs --exit-code 0 . && gitleaks detect --exit-code 1\necho gitleaks\ngitleaks --version\ngitleaks protect", gitleaks))
	assert.Empty(t, matcher.Commands("make lint", gitleaks))
}

func TestUnlessExceptsANameOrWorkflow(t *testing.T) {
	rule := &rules.Rule{ID: "unit", Regex: regexp.MustCompile(`(?i)\btests?\b`), Unless: regexp.MustCompile(`(?i)\bdeploy\b`),
		ApplyOn: []string{rules.FieldArtifactName, rules.FieldJobName}, Categories: []string{"Unit Tests"}}
	artifacts := []*models.Artifact{
		{Name: "deploy test", Jobs: []*models.Job{{Name: "deploy-test"}, {Name: "unit-tests"}}},
		{Name: "deploy test env"},
	}

	found := matcher.NewMatchService().Match(artifacts, []*rules.Rule{rule})
	require.Len(t, found, 1)
	assert.Equal(t, "unit-tests", found[0].Job.Name)
}

// A workflow named after its tool used to yield a finding with no step, which
// grading could not weaken and which outranked the step that runs the tool.
func TestAStepWinsOverTheWorkflowName(t *testing.T) {
	trivy := &rules.Rule{ID: "trivy-rule", Regex: regexp.MustCompile(`(?i)\btrivy\b`),
		ApplyOn: []string{rules.FieldArtifactName, rules.FieldJobRun}, Categories: []string{"SCA"}}
	step := &models.Job{Run: "trivy fs --exit-code 0 ."}

	found := matcher.NewMatchService().Match([]*models.Artifact{{Name: "Trivy scan", Jobs: []*models.Job{step, {Run: "make"}}}}, []*rules.Rule{trivy})
	assert.Len(t, found, 1)
	assert.Same(t, step, found[0].Job)

	found = matcher.NewMatchService().Match([]*models.Artifact{{Name: "Trivy scan", Jobs: []*models.Job{{Run: "./scripts/scan.sh"}}}}, []*rules.Rule{trivy})
	assert.Len(t, found, 1)
	assert.Nil(t, found[0].Job, "with no step to go on, the name is the evidence")
}

// Installing a tool, fetching it or asking its version is not running it,
// and such a step outranked a real run that was weakened.
func TestInstallingIsNotRunning(t *testing.T) {
	tool := regexp.MustCompile(`(?i)\b(?:bandit|gosec|eslint|grype|trivy|osv-scanner|ruff|cosign|snyk|syft)\b`)
	rule := &rules.Rule{ID: "tool", Regex: tool, ApplyOn: []string{rules.FieldJobRun, rules.FieldJobPackage}, Categories: []string{"X"}}
	for _, tt := range []struct {
		step models.Job
		runs bool
	}{
		{models.Job{Run: "pip install bandit"}, false},
		{models.Job{Run: "python3 -m pip install --upgrade bandit"}, false},
		{models.Job{Run: "uv tool install ruff"}, false},
		{models.Job{Run: "go install github.com/securego/gosec/v2/cmd/gosec@latest"}, false},
		{models.Job{Run: "npm install --save-dev eslint"}, false},
		{models.Job{Run: "sudo apt-get install -y trivy"}, false},
		{models.Job{Run: "curl -sSfL https://raw.githubusercontent.com/anchore/grype/main/install.sh | sh -s -- -b /usr/local/bin"}, false},
		{models.Job{Run: "trivy --version"}, false},
		{models.Job{Run: "osv-scanner version"}, false},
		{models.Job{Package: "aquasecurity/setup-trivy"}, false},
		{models.Job{Package: "sigstore/cosign-installer"}, false},
		{models.Job{Package: "snyk/actions/setup"}, false},
		{models.Job{Package: "anchore/sbom-action/download-syft"}, false},
		{models.Job{Run: "bandit -r src"}, true},
		{models.Job{Run: "pipx run bandit -r src"}, true},
		{models.Job{Run: "uv run ruff check ."}, true},
		{models.Job{Run: "npx eslint ."}, true},
		{models.Job{Run: "go run github.com/securego/gosec/v2/cmd/gosec ./..."}, true},
		{models.Job{Run: "trivy fs --version-check=false ."}, true},
		{models.Job{Package: "snyk/actions/node"}, true},
		{models.Job{Package: "aquasecurity/trivy-action"}, true},
	} {
		step := tt.step
		found := matcher.NewMatchService().Match([]*models.Artifact{{Jobs: []*models.Job{&step}}}, []*rules.Rule{rule})
		assert.Equal(t, tt.runs, len(found) == 1, "%+v", step)
	}
}
