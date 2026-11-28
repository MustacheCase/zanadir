package grading

import (
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/MustacheCase/zanadir/scanner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A package script named after its tool stands for its own command. The
// command that calls the script is not a plain run of the tool, so the
// script's "|| true" decides the grade.
func TestScriptNamedAfterItsToolGradesItsOwnCommand(t *testing.T) {
	for _, tt := range []struct {
		name, script, run string
		want              Verdict
	}{
		{"swallowed script", `"eslint": "eslint . || true"`, "npm run eslint", Advisory},
		{"enforcing script", `"eslint": "eslint ."`, "npm run eslint", Enforcing},
		{"yarn swallowed", `"eslint": "eslint . || true"`, "yarn eslint", Advisory},
		{"bun swallowed", `"eslint": "eslint . || true"`, "bun run eslint", Advisory},
		{"lint script still advisory", `"lint": "eslint . || true"`, "npm run lint", Advisory},
		{"lint script after a longer name", `"lint": "eslint . || true"`, "npm run lint:fix && npm run lint", Advisory},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := testrepo.Write(t, map[string]string{
				"package.json":             "{\n  \"scripts\": {\n    " + tt.script + "\n  }\n}\n",
				".github/workflows/ci.yml": "on: pull_request\njobs:\n  check:\n    steps:\n      - run: " + tt.run + "\n",
			})
			artifacts, err := scanner.NewRepositoryScanner().Scan(dir)
			require.NoError(t, err)
			require.NotEmpty(t, artifacts)
			require.NotEmpty(t, artifacts[0].Jobs)
			require.NotEmpty(t, artifacts[0].Jobs[0].Followed)
			rs, err := rules.NewRulesService()
			require.NoError(t, err)

			var findings []*matcher.Finding
			for _, c := range models.CategoryTitles {
				findings = append(findings, matcher.NewMatchService().Match(artifacts, rs.GetCategoryRules(c))...)
			}
			assert.Equal(t, tt.want, ByCategory(findings, nil)["Linter"].Verdict)
		})
	}
}

func TestFollowedTestWrappersKeepUnitTestCoverage(t *testing.T) {
	for _, tt := range []struct {
		name, config, run string
	}{
		{"npm test", "package.json", "npm test"},
		{"tox", "tox.ini", "tox"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{
				".github/workflows/ci.yml": "on: pull_request\njobs:\n  check:\n    steps:\n      - run: " + tt.run + "\n",
			}
			if tt.config == "package.json" {
				files[tt.config] = "{\n  \"scripts\": {\n    \"test\": \"node scripts/runner.js\"\n  }\n}\n"
			} else {
				files[tt.config] = "[testenv]\ncommands = python -m custom_runner\n"
			}
			dir := testrepo.Write(t, files)
			artifacts, err := scanner.NewRepositoryScanner().Scan(dir)
			require.NoError(t, err)
			rs, err := rules.NewRulesService()
			require.NoError(t, err)

			var findings []*matcher.Finding
			for _, c := range models.CategoryTitles {
				findings = append(findings, matcher.NewMatchService().Match(artifacts, rs.GetCategoryRules(c))...)
			}
			assert.Equal(t, Enforcing, ByCategory(findings, nil)["Unit Tests"].Verdict)
		})
	}
}
