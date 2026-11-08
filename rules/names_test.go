package rules_test

import (
	"slices"
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rules that cover several tools show what they matched instead of a name.
var namedByMatch = []string{
	"unit-test-command-rule", "coverage-command-rule", "python-type-checker-rule", "php-static-analysis-rule",
	"kotlin-linter-rule", "haskell-lua-linter-rule", "test-results-step-rule", "coverage-results-step-rule",
	"npm-audit-rule", "oxsecurity-super-linter-rule", "formatter-check-rule", "pre-commit-rule", "os-code-signing-rule",
}

func TestEmbeddedRulesAreNamed(t *testing.T) {
	rs, err := rules.NewRulesService()
	require.NoError(t, err)
	for _, category := range models.CategoryTitles {
		for _, r := range rs.GetCategoryRules(category) {
			if slices.Contains(namedByMatch, r.ID) {
				assert.Empty(t, r.Name, "%s is named by what it matches", r.ID)
				continue
			}
			assert.NotEmpty(t, r.Name, "%s needs a name for the report", r.ID)
		}
	}
}

func TestToolNamesWhatMatched(t *testing.T) {
	rs, err := rules.NewRulesService(rules.FileRule{
		ID: "acme-scan", Name: "Acme Scan", ApplyOn: []string{rules.FieldJobRun}, Categories: []string{"SAST"}, Regex: `\bacme\b`,
	}, rules.FileRule{
		ID: "acme-everything", ApplyOn: []string{rules.FieldJobRun}, Categories: []string{"SAST"}, Regex: `acme-run.*`,
	}, rules.FileRule{
		ID: "acme-blank", ApplyOn: []string{rules.FieldJobRun}, Categories: []string{"SAST"}, Regex: `\s+`,
	})
	require.NoError(t, err)

	for _, tt := range []struct{ rule, text, tool string }{
		{"nancy-rule", "nancy sleuth", "nancy"},
		{"acme-scan", "acme --all", "Acme Scan"},
		{"acme-everything", "acme-run --target production --region eu-west-1 --verbose", "acme-run --target production --region e…"},
		{"acme-blank", "a  b", "acme-blank"},
		{"unit-test-command-rule", "go test ./...", "go test"},
		{"unit-test-command-rule", "npm run test", "npm run test"},
		{"unit-test-command-rule", "mvn -B -ntp clean verify", "mvn"},
		{"unit-test-command-rule", "./gradlew build check", "gradlew"},
		{"unit-test-command-rule", "python3 -m unittest discover", "unittest"},
		{"unit-test-command-rule", "pytest -q tests", "pytest"},
		{"unit-test-command-rule", "(bats tests | tee out.txt)", "bats"},
		{"unit-test-command-rule", "./test/bats/bin/bats --tap test", "bats"},
		{"unit-test-command-rule", "npx playwright test", "playwright test"},
		{"unit-test-command-rule", "node_modules/.bin/playwright test --shard=1/4", "playwright test"},
		{"unit-test-command-rule", "docker run --rm -v $(pwd):/apps helmunittest/helm-unittest .", "helmunittest/helm-unittest"},
		{"unit-test-command-rule", "./bin/gotestsum --format testname", "gotestsum"},
		{"unit-test-command-rule", "go run gotest.tools/gotestsum@v1.13.0 --rerun-fails=2 --packages=./... -- -v", "gotestsum"},
		{"coverage-command-rule", "pytest --cov=app tests", "pytest"},
		{"coverage-command-rule", "go  test -coverprofile=c.out ./...", "go test"},
		{"coverage-command-rule", "coverage run -m pytest", "coverage"},
		{"coverage-command-rule", "npx jest --coverage", "jest"},
		{"coverage-command-rule", "phpunit --coverage-clover clover.xml", "phpunit"},
		{"coverage-command-rule", "nyc mocha", "nyc"},
		{"python-type-checker-rule", "mypy src", "mypy"},
		{"python-type-checker-rule", "uv run ty check", "ty"},
		{"npm-audit-rule", "pnpm audit --prod", "pnpm audit"},
		{"oxsecurity-super-linter-rule", "super-linter/super-linter/slim@v7", "super-linter"},
		{"oxsecurity-super-linter-rule", "oxsecurity/megalinter@v8", "megalinter"},
		{"test-results-step-rule", "PublishTestResults", "PublishTestResults"},
		{"test-results-step-rule", "EnricoMi/publish-unit-test-result-action/linux", "publish-unit-test-result-action"},
		{"unit-test-command-rule", "mvn -B clean install", "mvn"},
		{"formatter-check-rule", "cargo fmt --all -- --check", "cargo fmt"},
		{"formatter-check-rule", "prettier@3.9.0 --check .", "prettier"},
		{"formatter-check-rule", "terraform fmt -check -recursive", "terraform fmt"},
		{"formatter-check-rule", "./gradlew spotlessCheck", "spotless"},
		{"formatter-check-rule", "dprint/check", "dprint"},
		{"pre-commit-rule", "pre-commit/action", "pre-commit"},
		{"pre-commit-rule", "prek --all-files", "prek"},
		{"os-code-signing-rule", `codesign -f -s "$IDENTITY" App.app`, "codesign"},
		{"os-code-signing-rule", "xcrun notarytool submit App.dmg --wait", "notarytool"},
		// Text the rule does not match leaves only its id.
		{"unit-test-command-rule", "make", "unit-test-command-rule"},
	} {
		assert.Equal(t, tt.tool, findRule(t, rs, tt.rule).Tool(tt.text), "%s on %q", tt.rule, tt.text)
	}
}
