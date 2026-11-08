package rules_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/stretchr/testify/assert"
)

// findRule looks a rule up by id across every category.
func findRule(t *testing.T, rs rules.RuleService, id string) *rules.Rule {
	t.Helper()
	for _, category := range models.CategoryTitles {
		for _, r := range rs.GetCategoryRules(category) {
			if r.ID == id {
				return r
			}
		}
	}
	t.Fatalf("rule %q not found", id)
	return nil
}

// The regexes were bare substrings. The shouldNotMatch cases are lookalikes
// that used to produce a false "this category is covered".
func TestRuleRegexPrecision(t *testing.T) {
	rs, err := rules.NewRulesService()
	assert.NoError(t, err)

	tests := []struct {
		ruleID         string
		shouldMatch    []string
		shouldNotMatch []string
	}{
		{
			ruleID: "github-action-benchmark-rule",
			shouldMatch: []string{
				"benchmark-action/github-action-benchmark@v1",
				"benchmark-action/github-action-benchmark",
				"rhysd/github-action-benchmark@v1",
			},
			shouldNotMatch: []string{
				"cargo bench", "go test -bench=. ./...", "CodSpeedHQ/action@v3",
				"benchmark-action/github-action-benchmark-extra@v1",
			},
		},
		{
			ruleID: "guarddog-rule",
			shouldMatch: []string{
				"uvx guarddog pypi verify requirements.txt --exit-non-zero-on-finding",
				"uvx guarddog@2.8.0 pypi verify requirements.txt --exit-non-zero-on-finding",
				"guarddog npm scan express",
				"guarddog github_action verify .github/workflows/ci.yml --output-format sarif",
				"docker run --rm ghcr.io/datadog/guarddog pypi scan requests",
				"docker run --rm ghcr.io/datadog/guarddog:2.8.0 npm verify package.json",
				"docker run ghcr.io/datadog/guarddog@sha256:abcdef pypi scan requests",
				"./bin/guarddog go verify go.mod",
				"guarddog rubygems scan rails",
				"guarddog --log-level debug npm scan express",
				"guarddog --log-level=DEBUG pypi verify requirements.txt",
			},
			shouldNotMatch: []string{
				"guarddog pypi list-rules", "guarddog npm list-rules", "python -m watchdog.watchmedo",
				"docker run ghcr.io/datadog/guarddog:2.8.0 npm list-rules",
				"myguarddog npm scan express", "guarddogger npm scan express", "my-guarddog npm scan express",
				"guarddog npm scan-extra express", "guarddog pypi verify-extra requirements.txt",
				"guarddog", "guarddog --help", "ghcr.io/datadog/guarddog",
			},
		},
		{
			ruleID: "cfn-guard-rule",
			shouldMatch: []string{
				"cfn-guard validate -d template.yaml -r rules.guard",
				"cfn-guard validate --data templates/ --rules guard/ --show-summary fail",
				"/usr/src/cloudformation-guard/cfn-guard validate -r rules/ -d templates/ --output-format sarif",
				"~/.guard/bin/cfn-guard validate --data cfn/ --rules rules/",
				"set -e; cfn-guard validate -d template.yaml -r rules.guard",
				"CFN-GUARD VALIDATE -d template.yaml -r rules.guard",
			},
			shouldNotMatch: []string{
				"cfn-guard test -r rules.guard -t tests.yaml", "cfn-guard rulegen --template template.yaml",
				"cfn-guard parse-tree -r rules.guard", "cfn-lint template.yaml",
				"my-cfn-guard validate -d template.yaml", "cfn-guard validate-rules -r rules.guard",
				"cfn-guard", "aws cloudformation validate-template --template-body file://template.yaml",
			},
		},
		{
			ruleID:         "trivy-rule",
			shouldMatch:    []string{"trivy fs .", "trivy image --exit-code 1 alpine", "trivy -q repo .", "trivy --cache-dir /tmp/t rootfs /", "trivy sbom sbom.json", "docker run aquasec/trivy filesystem /src"},
			shouldNotMatch: []string{"trivyscanner", "mytrivy fs", "trivy config .", "trivy --version", "trivy image-something", "aquasecurity/trivy-action"},
		},
		{
			ruleID:         "sonar-rule",
			shouldMatch:    []string{"SonarCloudAnalyze", "SonarQubeAnalyze", "sonarsource/sonarcloud-github-action", "sonar-scanner", "withSonarQubeEnv", "waitForQualityGate"},
			shouldNotMatch: []string{"SonarCloudPrepare", "SonarQubePublish", "sonarlint"},
		},
		{
			ruleID:         "snyk-rule",
			shouldMatch:    []string{"SnykSecurityScan", "snyk test", "snyk/snyk-scan", "snykSecurity"},
			shouldNotMatch: []string{"snykish", "SnykSecurityScanner"},
		},
		{
			ruleID:         "azure-dependency-scanning-rule",
			shouldMatch:    []string{"AdvancedSecurity-Dependency-Scanning"},
			shouldNotMatch: []string{"AdvancedSecurity-Codeql-Analyze", "AdvancedSecurity-Dependency-Scanning-Extra"},
		},
		{
			ruleID:         "coverage-results-step-rule",
			shouldMatch:    []string{"PublishCodeCoverageResults", "recordCoverage", "publishCoverage", "coverage_report"},
			shouldNotMatch: []string{"PublishTestResults", "PublishCodeCoverageResultsV2Wrapper", "coverage_report_upload"},
		},
		{
			ruleID:         "test-results-step-rule",
			shouldMatch:    []string{"VSTest", "PublishTestResults", "junit"},
			shouldNotMatch: []string{"VSBuild", "PublishTestResultsLegacy"},
		},
		{
			ruleID:         "gitlab-sast-template-rule",
			shouldMatch:    []string{"Jobs/SAST.gitlab-ci.yml", "Security/SAST.gitlab-ci.yml", "Jobs/SAST.latest.gitlab-ci.yml", "$CI_SERVER_FQDN/components/sast/sast@2.0.2"},
			shouldNotMatch: []string{"Jobs/SAST-IaC.gitlab-ci.yml", "Security/DAST.gitlab-ci.yml", "my/SAST.gitlab-ci.yml"},
		},
		{
			ruleID:         "gitlab-sast-iac-template-rule",
			shouldMatch:    []string{"Jobs/SAST-IaC.gitlab-ci.yml", "Security/SAST-IaC.latest.gitlab-ci.yml"},
			shouldNotMatch: []string{"Jobs/SAST.gitlab-ci.yml"},
		},
		{
			ruleID:         "gitlab-secret-detection-template-rule",
			shouldMatch:    []string{"Jobs/Secret-Detection.gitlab-ci.yml", "Security/Secret-Detection.gitlab-ci.yml", "gitlab.com/components/secret-detection/secret-detection@1"},
			shouldNotMatch: []string{"Jobs/SAST.gitlab-ci.yml"},
		},
		{
			ruleID:         "gitlab-dependency-scanning-template-rule",
			shouldMatch:    []string{"Jobs/Dependency-Scanning.latest.gitlab-ci.yml", "Security/Dependency-Scanning.gitlab-ci.yml", "Jobs/Dependency-Scanning.v2.gitlab-ci.yml", "Security/Container-Scanning.gitlab-ci.yml", "$CI_SERVER_FQDN/components/container-scanning/container-scanning@6"},
			shouldNotMatch: []string{"Jobs/Code-Quality.gitlab-ci.yml", "Security/DAST.gitlab-ci.yml"},
		},
		{
			ruleID:         "gitlab-license-scanning-template-rule",
			shouldMatch:    []string{"Security/License-Scanning.gitlab-ci.yml"},
			shouldNotMatch: []string{"Security/Secret-Detection.gitlab-ci.yml"},
		},
		{
			ruleID:         "gitlab-code-quality-template-rule",
			shouldMatch:    []string{"Jobs/Code-Quality.gitlab-ci.yml", "Code-Quality.gitlab-ci.yml"},
			shouldNotMatch: []string{"Jobs/Code-Intelligence.gitlab-ci.yml"},
		},
		{
			ruleID:         "gitlab-performance-template-rule",
			shouldMatch:    []string{"Jobs/Browser-Performance-Testing.gitlab-ci.yml", "Verify/Load-Performance-Testing.gitlab-ci.yml", "Verify/Browser-Performance.gitlab-ci.yml"},
			shouldNotMatch: []string{"Security/DAST.gitlab-ci.yml"},
		},
		{
			ruleID:         "dependency-check-rule",
			shouldMatch:    []string{"dependencyCheck", "dependencyCheckPublisher", "dependency-check.sh --scan .", "dependency-check --project x"},
			shouldNotMatch: []string{"dependencyCheckBomName", "my-dependency-checker"},
		},
		{
			ruleID:         "composer-audit-rule",
			shouldMatch:    []string{"composer audit", "composer audit --locked --no-dev", "php composer.phar audit", "composer --no-interaction audit --format=json"},
			shouldNotMatch: []string{"composer install", "composer update", "composer validate", "composer run-script audit", "composer audit-fix"},
		},
		{
			ruleID:         "jenkins-record-issues-rule",
			shouldMatch:    []string{"recordIssues"},
			shouldNotMatch: []string{"recordIssuesLater"},
		},
		{
			ruleID:         "git-secrets-rule",
			shouldMatch:    []string{"atlassian/git-secrets-scan", "git secrets --scan", "git-secrets --scan-history"},
			shouldNotMatch: []string{"git-secretsmanager", "github secrets", "git secret reveal"},
		},
		{
			ruleID:         "trivy-eol-rule",
			shouldMatch:    []string{"trivy image --exit-on-eol 1 zanadir:eol-scan", "exit-on-eol: 1"},
			shouldNotMatch: []string{"trivy fs .", "aquasecurity/trivy-action", "trivy sbom"},
		},
		{
			// Scout is used for CVEs far more often than for base-image age, so
			// the bare command must not count as end-of-life coverage.
			ruleID:         "docker-scout-eol-rule",
			shouldMatch:    []string{"docker scout policy --only-eol", "scout recommendations --eol"},
			shouldNotMatch: []string{"docker scout cves", "docker/scout-action", "scouting the eol", "eol scout"},
		},
		{
			// The bots are reported by the dependency-bot parser with the tool
			// name as the whole package. dependabot/fetch-metadata is a real
			// action for reading pull request metadata, and repositories have
			// entire workflows named dependabot-triage; neither is dependency
			// scanning, so the regex is anchored.
			ruleID:         "dependabot-rule",
			shouldMatch:    []string{"dependabot"},
			shouldNotMatch: []string{"dependabot/fetch-metadata", "dependabot-triage", "my-dependabot", "dependabot-core"},
		},
		{
			ruleID:         "renovate-rule",
			shouldMatch:    []string{"renovate"},
			shouldNotMatch: []string{"renovatebot/github-action", "renovate-config", "auto-renovate"},
		},
		{
			ruleID:         "mix-audit-rule",
			shouldMatch:    []string{"mix deps.audit", "mix hex.audit", "mix do deps.get, deps.audit", "mix do deps.get + hex.audit", "mix deps.audit --ignore-file .mix-audit-skips", "mix_audit"},
			shouldNotMatch: []string{"mix deps.get", "mix test", "mix audit_log", "mix deps.auditor", "echo mix_audit_report"},
		},
		{
			ruleID:         "bouncer-rule",
			shouldMatch:    []string{"bouncer check ./...", ".tool/bouncer list", "github.com/wagoodman/go-bouncer"},
			shouldNotMatch: []string{"bouncer", "deploy-bouncer-service", "bouncercheck"},
		},
		{
			ruleID:         "ruff-rule",
			shouldMatch:    []string{"uv run ruff check .", "astral-sh/ruff-action", "ruff format --check"},
			shouldNotMatch: []string{"ruffle", "gruff"},
		},
		{
			// "ty" is two letters that turn up inside words and paths, so only
			// "ty check" counts, and "ty checkout" is not the sub-command.
			ruleID: "python-type-checker-rule",
			shouldMatch: []string{
				"uv run mypy --config-file ./pyproject.toml src", "pyright", "jakebailey/pyright-action",
				"ty check", "uv run ty check", "uv run --locked ty check", "uvx ty check",
				"uvx ty@latest check src", "uv run --python 3.15 --with ty ty check --output-format=github",
				"ty check --exit-zero-on-warning --output-format=github", "ty check openbb_ecb",
				"uv run --only-group check ty check python/uv",
			},
			shouldNotMatch: []string{
				"mypyc-build", "pyrightish",
				"ty version", "ty server", "ty explain rule unresolved-import", "uv run ty", "ty checkout",
				"pretty check", "./scripts/ty-check.sh", "cargo build --profile=profiling --bin=ty",
				"mv target/profiling/ty ty-new",
			},
		},
		{
			ruleID: "coverage-command-rule",
			shouldMatch: []string{
				"uv run pytest --cov vunnel -v tests/unit/", "pytest tests --cov=src --cov-report=xml",
				"coverage run -m unittest", "go test ./... -coverprofile=coverage.txt", "go test -race -cover ./...",
				"npx jest --coverage", "vitest run --coverage", "nyc mocha", "cargo llvm-cov --lcov",
				"cargo tarpaulin --out Xml", "vendor/bin/phpunit --coverage-clover clover.xml",
				`dotnet test --collect:"XPlat Code Coverage"`, "vladopajic/go-test-coverage",
				"python -m coverage xml", "coverage combine", "diff-cover coverage.xml --compare-branch=main",
			},
			shouldNotMatch: []string{
				"pytest tests/", "go test ./...", "go test -run TestCoverage ./...", "jest", "npm run coverage-badge",
				"pytest --covid", "go test -coverage-of-nothing",
			},
		},
		{
			ruleID: "grype-rule",
			shouldMatch: []string{"grype dir:.", "anchore/grype", "./bin/grype sbom:sbom.json --fail-on high",
				"docker run anchore/grype:latest alpine", "set -e\ngrype .", "anchore/grype@v0.80.0",
				"$CI_SERVER_HOST/proton/devops/cicd-components/devsecops/grype/scan-code@~latest"},
			shouldNotMatch: []string{"grypeless", "go run cmd/syft/main.go anchore/test_images:grype-quality-dotnet-69f15d2",
				"grype-db build", "cat .grype.yaml"},
		},
		{
			ruleID:         "zizmor-rule",
			shouldMatch:    []string{"zizmor .github/workflows", "pipx run zizmor", "crazy-max/.github/.github/workflows/zizmor.yml"},
			shouldNotMatch: []string{"zizmorish", "myzizmor"},
		},
		{
			ruleID:         "zizmor-action-rule",
			shouldMatch:    []string{"zizmorcore/zizmor-action", "ZizmorCore/Zizmor-Action"},
			shouldNotMatch: []string{"zizmorcore/zizmor-action-fork", "zizmor .github/workflows"},
		},
		{
			// The organisation is also called step-security, and its other
			// actions and forks of this one are not it. Job.Package arrives
			// without the @version, so the match can be anchored at both ends.
			ruleID: "harden-runner-rule",
			shouldMatch: []string{
				"step-security/harden-runner", "Step-Security/Harden-Runner",
			},
			shouldNotMatch: []string{
				"step-security/changed-files", "step-security/wait-for-secrets",
				"step-security/harden-runner-fork", "my-org/step-security/harden-runner",
			},
		},
		{
			ruleID: "glsec-rule",
			shouldMatch: []string{
				"ghcr.io/glsec/glsec",
				"gitlab.com/glsec-io/glsec/glsec@v1.0.25",
				"gitlab.com/glsec-io/glsec/glsec-code-quality@v1.0.25",
				"gitlab.com/GLSEC-IO/glsec/glsec-code-quality@1.23.0",
				"$CI_SERVER_FQDN/glsec-io/glsec/glsec@v1.0.25",
				"ghcr.io/glsec/glsec:1.23.0",
				"ghcr.io/glsec/glsec@sha256:abc",
				"glsec --strict .gitlab-ci.yml",
				"./glsec .gitlab-ci.yml",
				"glsec",
				"docker run --rm ghcr.io/glsec/glsec:1.23.0 .gitlab-ci.yml",
			},
			shouldNotMatch: []string{
				"glsecure", "myglsec", "glsec_tool", "gl-sec",
				// Components and images that only have glsec in their path.
				"gitlab.com/someone/glsec/glsec@1",
				"gitlab.com/glsec-io/glsec/glsec-extra@1",
				"ghcr.io/glsec/glsec-fork:1",
				"registry.example.com/glsec:1",
				"curl -sSLO https://github.com/glsec/glsec/releases/download/v1.23.0/glsec_1.23.0_linux_amd64.tar.gz",
			},
		},
		{
			ruleID: "poutine-rule",
			shouldMatch: []string{
				"boostsecurityio/poutine-action",
				"poutine analyze_local .",
				"poutine analyze_repo org/repo --token $GH_TOKEN",
				"poutine analyze_org my-org --scm gitlab",
				"docker run --rm -v $PWD:/src ghcr.io/boostsecurityio/poutine:1.1.6 analyze_local . --fail-on-violation",
				"ghcr.io/boostsecurityio/poutine:1.1.6",
			},
			shouldNotMatch: []string{"poutine", "make poutine", "echo poutine recipe", "poutineries analyze", "boostsecurityio/poutines"},
		},
		{
			ruleID: "skillspector-rule",
			shouldMatch: []string{
				"skillspector scan .claude/skills --no-llm --format sarif --output skills.sarif",
				"uvx --from git+https://github.com/NVIDIA/skillspector.git@v2.13.1 skillspector scan ./skills",
				"docker run --rm -v $PWD:/src skillspector:2.13.1 scan /src/skills",
			},
			shouldNotMatch: []string{
				"uv run skillspector --version", "docker build -t skillspector .",
				"uv tool install git+https://github.com/NVIDIA/skillspector.git", "skillspectors scan", "skillspector baseline ./skill",
			},
		},
		{
			ruleID: "cisco-skill-scanner-rule",
			shouldMatch: []string{
				"cisco-ai-defense/skill-scanner/.github/workflows/scan-skills.yml",
				"skill-scanner scan-all ./skills --recursive --fail-on-severity high",
				"skill-scanner scan ./skill",
				"uvx --from cisco-ai-skill-scanner skill-scanner scan-repo owner/repo",
				"pipx run --spec cisco-ai-skill-scanner==2.3.1 skill-scanner scan-all . --recursive",
			},
			shouldNotMatch: []string{
				"npx skills add https://github.com/getsentry/skills --skill skill-scanner",
				"skill-scanner",
				"my-skill-scanner scan .",
				"skill-scanner --version",
			},
		},
		{
			ruleID: "snyk-agent-scan-rule",
			shouldMatch: []string{
				"uvx snyk-agent-scan@0.6.9 --ci --dangerously-run-mcp-servers .claude/skills",
				"snyk-agent-scan scan ./skills/deploy/SKILL.md",
			},
			shouldNotMatch: []string{"snyk test", "agent-scan", "snyk-agent-scanner"},
		},
		{
			ruleID:         "actionlint-rule",
			shouldMatch:    []string{"rhysd/actionlint", "reviewdog/action-actionlint", "actionlint -color"},
			shouldNotMatch: []string{"actionlinter", "action lint"},
		},
		{
			// The plural is a different thing, and the boundary excludes it.
			ruleID:         "scorecard-rule",
			shouldMatch:    []string{"ossf/scorecard-action", "Scorecard analysis", "scorecard --repo=."},
			shouldNotMatch: []string{"scorecards", "scorecarding"},
		},
		{
			ruleID:         "govulncheck-rule",
			shouldMatch:    []string{"govulncheck ./...", "golang/govulncheck-action", "go run golang.org/x/vuln/cmd/govulncheck@latest ./..."},
			shouldNotMatch: []string{"govulncheckers", "mygovulncheck"},
		},
		{
			ruleID:         "bundler-audit-rule",
			shouldMatch:    []string{"bundle-audit check --update", "bundle exec bundle-audit", "bundle audit check", "bundler-audit", "bundle exec rake bundle:audit"},
			shouldNotMatch: []string{"bundle install", "bundle exec rspec", "bundle exec rubocop --audit-log x"},
		},
		{
			ruleID:         "osv-scanner-rule",
			shouldMatch:    []string{"osv-scanner -r .", "google/osv-scanner-action"},
			shouldNotMatch: []string{"osv-scannerx", "osv database"},
		},
		{
			ruleID:         "pip-audit-rule",
			shouldMatch:    []string{"pip-audit -r requirements.txt", "pypa/gh-action-pip-audit"},
			shouldNotMatch: []string{"pip-auditor", "pip install audit"},
		},
		{
			// "safety" is a common word, so only its sub-commands and action.
			ruleID: "safety-rule",
			shouldMatch: []string{
				"safety check -r requirements.txt", "safety scan", "pyupio/safety-action@v1",
				"python -m safety check", "safety --key $SAFETY_API_KEY scan", "safety --disable-optional-telemetry check --full-report",
			},
			shouldNotMatch: []string{"tox -e safety", "type-safety", "safety-first", "type-safety check", "pip install safety", "safety --version", "PR Tooling Safety Check", "Safety Check"},
		},
		{
			ruleID:         "cargo-audit-rule",
			shouldMatch:    []string{"cargo audit", "cargo audit --deny warnings", "rustsec/audit-check@v2", "actions-rs/audit-check@v1"},
			shouldNotMatch: []string{"cargo auditable build", "cargo install cargo-auditable", "cargo build", "npm audit"},
		},
		{
			// "retire" is an ordinary word, so only a command that runs it.
			ruleID: "retire-js-rule",
			shouldMatch: []string{
				"npx retire", "retire --path dist/ --severity high", "retire --outputformat json --exitwith 1",
				"npx --yes retire@5", "pnpm dlx retire", "node_modules/.bin/retire --path .", "lint\nretire",
			},
			shouldNotMatch: []string{"retired", "npm run retire-old-builds", "git rm retire.txt", "echo retire", "retire-js"},
		},
		{
			// "nancy" is also a person's name, so only the scanning
			// sub-commands and the action count.
			ruleID: "nancy-rule",
			shouldMatch: []string{
				"go list -json -deps ./... | nancy sleuth", "nancy sleuth --loud",
				"nancy sleuth --exclude-vulnerability-file .nancy-ignore", "nancy lifecycle", "nancy iq",
				"./bin/nancy sleuth", "docker run --rm -i sonatypecommunity/nancy:latest sleuth",
				"nancy -q sleuth", "nancy --skip-update-check sleuth",
				"nancy --guide-token $GUIDE_TOKEN sleuth", "nancy -u $USER -t $TOKEN sleuth",
				"sonatype-nexus-community/nancy-github-action@v1.0.2",
			},
			shouldNotMatch: []string{
				"nancy config", "nancy update", "./scripts/notify-nancy.sh",
				"kubectl rollout restart deploy/nancy-api", "nancy", "nancysleuth", "nancy sleuthing",
				"docker run -i sonatypecommunity/nancy:latest", "nancy -q config", "nancy -u x update",
			},
		},
		{
			// "audit" on its own is in half the workflow names on GitHub, so
			// only the subcommand counts.
			ruleID:         "npm-audit-rule",
			shouldMatch:    []string{"npm audit", "npm audit --audit-level=high", "yarn audit", "pnpm audit --prod"},
			shouldNotMatch: []string{"npm auditor", "audit", "security audit job", "npm install", "npm run audit-ci"},
		},
		{
			ruleID:         "snyk-rule",
			shouldMatch:    []string{"snyk test", "snyk/actions/golang"},
			shouldNotMatch: []string{"snyking"},
		},
		{
			// Verifying registry signatures is a supply-chain check, not SCA.
			ruleID:         "npm-audit-signatures-rule",
			shouldMatch:    []string{"npm audit signatures", "npm audit signatures --omit=dev"},
			shouldNotMatch: []string{"npm audit", "npm audit --audit-level=high", "npm audit fix", "npm audit signature"},
		},
		{
			ruleID:         "gitleaks-rule",
			shouldMatch:    []string{"gitleaks detect --source .", "gitleaks/gitleaks-action"},
			shouldNotMatch: []string{"gitleaksy"},
		},
		{
			ruleID:         "trufflehog-rule",
			shouldMatch:    []string{"trufflehog git file://.", "trufflesecurity/trufflehog"},
			shouldNotMatch: []string{"trufflehogsx"},
		},
		{
			ruleID:         "detect-secrets-rule",
			shouldMatch:    []string{"detect-secrets scan", "detect_secrets scan"},
			shouldNotMatch: []string{"detect-secretsfoo"},
		},
		{
			// "grant" is an English word and a SQL keyword.
			ruleID:         "grant-rule",
			shouldMatch:    []string{"grant check", "anchore/grant", "grant list"},
			shouldNotMatch: []string{`psql -c "GRANT ALL PRIVILEGES ON db TO user"`, "grants-management", "grant the token access"},
		},
		{
			ruleID:         "golangci-lint-rule",
			shouldMatch:    []string{"golangci-lint run ./...", "golangci/golangci-lint-action"},
			shouldNotMatch: []string{"mygolangci-lintx"},
		},
		{
			// "revive" is an English word; \b alone matches revive-db.sh.
			ruleID:         "revive-rule",
			shouldMatch:    []string{"revive -set_exit_status ./...", "revive -config revive.toml -formatter friendly ./...", "go run github.com/mgechev/revive@latest ./...", "mgechev/revive"},
			shouldNotMatch: []string{"golangci-lint run", "./scripts/revive-db.sh", "revived", "Revive stale issues"},
		},
		{
			ruleID:         "eslint-rule",
			shouldMatch:    []string{"eslint .", "npx eslint --fix"},
			shouldNotMatch: []string{"eslintrc", "myeslint"},
		},
		{
			// "latest" contains "test", which ".*test.*" matched.
			ruleID:         "unit-test-rule",
			shouldMatch:    []string{"test", "unit-tests", "run-tests", "go test ./...", "build-and-test", "go-test-race", "release-test"},
			shouldNotMatch: []string{"latest", "deploy-latest", "protest-build", "contest"},
		},
		{
			// A bare "ab" matched any standalone occurrence.
			ruleID:         "apache-bench-rule",
			shouldMatch:    []string{"apache-bench -n 100", "apachebench", "ab -n 1000 http://x/"},
			shouldNotMatch: []string{"ab testing", "build ab", "grab a coffee"},
		},
		{
			ruleID:      "blazemeter-rule",
			shouldMatch: []string{"docker run blazemeter/taurus test.yml"},
		},
		{
			ruleID:         "taurus-rule",
			shouldMatch:    []string{"bzt load-test.yml", "bzt -o modules.jmeter.properties.threads=10 test.yml", "python -m bzt test.yml", "~/.local/bin/bzt", "bzt"},
			shouldNotMatch: []string{"tar -xjf archive.tar.bz2", "coverage run --source=bzt -m nose2", "bzt.yml"},
		},
		{
			ruleID:         "semgrep-rule",
			shouldMatch:    []string{"semgrep ci", "semgrep/semgrep-action", "semgrep scan --config auto"},
			shouldNotMatch: []string{"semgreppy", "mysemgrep"},
		},
		{
			ruleID: "flawfinder-rule",
			shouldMatch: []string{"flawfinder --error-level=4 src/", "flawfinder --minlevel 3 --sarif . > flawfinder.sarif",
				"pipx run flawfinder --error-level 1 lib/", "david-a-wheeler/flawfinder"},
			shouldNotMatch: []string{"cppcheck src/", "./scripts/find-flaws.sh", "flawfinders"},
		},
		{
			ruleID: "codeql-rule",
			shouldMatch: []string{"codeql database create --language=go,actions", "codeql database create",
				"codeql database analyze db --format=sarif-latest", "AdvancedSecurity-Codeql-Init@1"},
			shouldNotMatch: []string{"codeqlish", "github/codeql-action/upload-sarif@v3", "github/codeql-action/analyze@v3",
				"echo codeql"},
		},
		{
			ruleID:         "codeql-name-rule",
			shouldMatch:    []string{"CodeQL", "CodeQL Advanced", "Run CodeQL Analysis"},
			shouldNotMatch: []string{"codeqlish"},
		},
		{
			// A bare "sonar" appears in unrelated project and job names.
			ruleID:         "sonar-rule",
			shouldMatch:    []string{"SonarSource/sonarqube-scan-action", "sonar-scanner -Dsonar.host.url=x", "sonarcloud scan"},
			shouldNotMatch: []string{"sonar", "sonarian", "build-sonar-module"},
		},
		{
			ruleID:         "gosec-rule",
			shouldMatch:    []string{"gosec ./...", "securego/gosec@master"},
			shouldNotMatch: []string{"gosecure", "mygosecx"},
		},
		{
			ruleID:         "bandit-rule",
			shouldMatch:    []string{"bandit -r .", "PyCQA/bandit"},
			shouldNotMatch: []string{"banditry", "mybandits"},
		},
		{
			// "snyk" alone is the SCA rule; SAST is specifically "snyk code".
			ruleID:         "snyk-code-rule",
			shouldMatch:    []string{"snyk code test", "snyk  code  test --json"},
			shouldNotMatch: []string{"snyk test", "snyk monitor", "snyk container test"},
		},
		{
			// "Bearer" is ubiquitous in auth headers and must never match alone.
			ruleID:         "bearer-rule",
			shouldMatch:    []string{"bearer/bearer-action@v2", "bearer scan ."},
			shouldNotMatch: []string{"Authorization: Bearer ${{ secrets.GITHUB_TOKEN }}", "bearer token", "-H \"Authorization: Bearer abc\""},
		},
		{
			ruleID:         "checkov-rule",
			shouldMatch:    []string{"checkov -d .", "bridgecrewio/checkov-action@master"},
			shouldNotMatch: []string{"checkovx", "mycheckov"},
		},
		{
			ruleID:         "tfsec-rule",
			shouldMatch:    []string{"tfsec .", "aquasecurity/tfsec-action"},
			shouldNotMatch: []string{"tfsecure", "mytfsec"},
		},
		{
			ruleID:         "conftest-rule",
			shouldMatch:    []string{"conftest test deploy/", "conftest test --policy policy/ k8s/*.yaml", "docker run openpolicyagent/conftest test .", "conftest --no-color test -o github main.tf", "helm template . | conftest test -"},
			shouldNotMatch: []string{"go test ./...", "npm run conftest-docs", "conftest verify --policy policy/", "conftest pull oci://registry/policy", "conftest --version", "conftest testdata"},
		},
		{
			ruleID:         "kics-rule",
			shouldMatch:    []string{"kics scan -p .", "checkmarx/kics-github-action"},
			shouldNotMatch: []string{"kicks", "nkics"},
		},
		{
			ruleID:         "kube-score-rule",
			shouldMatch:    []string{"kube-score score deploy/*.yaml", "helm template . | kube-score score -", "kube-score score --exit-one-on-warning k8s/", "docker run -v $PWD:/project zegl/kube-score score /project/k8s/*.yaml", "kube-score score"},
			shouldNotMatch: []string{"kubectl score", "kube-scorecard", "kube-score version", "kube-score help score", "kube-score completion bash", "kube-scorecard score k8s/"},
		},
		{
			// "snyk" alone is SCA; only the iac sub-command scans infrastructure.
			ruleID:         "snyk-iac-rule",
			shouldMatch:    []string{"snyk iac test", "snyk  iac  test ."},
			shouldNotMatch: []string{"snyk test", "snyk code test"},
		},
		{
			// "trivy" alone is SCA; the config sub-command is the misconfig scanner.
			ruleID:         "trivy-config-rule",
			shouldMatch:    []string{"trivy config .", "trivy  config  ./infra", "trivy -q conf .", "trivy fs --scanners vuln,misconfig .", "trivy repo --security-checks config ."},
			shouldNotMatch: []string{"trivy fs .", "trivy image alpine", "trivy fs --scanners vuln ."},
		},
		{
			ruleID:         "codecov-rule",
			shouldMatch:    []string{"codecov/codecov-action", "bash <(curl -s https://codecov.io/bash)"},
			shouldNotMatch: []string{"mycodecovx"},
		},
		{
			// A bare \btests?\b would count `if test -f` as coverage.
			ruleID: "unit-test-command-rule",
			shouldMatch: []string{
				"go test ./...", "go test -race -cover ./...", "npm test", "npm run test",
				"yarn test", "pnpm run test", "pytest -q", "cargo test --all",
				"dotnet test", "mvn -B verify test", "gradle clean test",
				"bundle exec rspec", "npx jest --ci", "mocha spec/", "vitest run", "tox -e py311",
				"./gradlew test", "./mvnw -B test", "gradle test -x integrationTest", "tox -e lint,py312", "tox",
				"mvn -Dmaven.test.skip=false test",
				"meson test -C _build", ".gitlab-ci/run-tests.sh _build x11", "./scripts/run_tests.sh",
				"mvn -B verify", "./mvnw verify -Pci", "./gradlew check", "python -m unittest discover", "python3.12 -m unittest",
				"bun test", "deno test -A", "swift test --parallel",
				"cargo nextest run --workspace", "cargo nextest run --profile ci --no-fail-fast", "cargo nextest r --workspace",
				"bats test/", "./test/bats/bin/bats --tap test", "npx bats tests",
				"time bats test", "(bats tests | tee out.txt)", "xargs bats", "make lint\nbats --tap test",
				"./vendor/bin/pest", "vendor/bin/pest --ci --parallel", "php artisan test --parallel",
				"composer test", "composer run test", "composer run-script test",
				"mvn -B clean install", "./mvnw -B package -Pci", "mvn deploy -s settings.xml", "./gradlew build", "./gradlew :app:build --scan",
				"npx playwright test", "pnpm exec playwright test --shard=1/4", "pnpm -C e2e exec playwright test -c playwright.visual.config.ts",
				"bunx playwright test --config e2e/playwright.config.ts", "yarn playwright test --project=chromium",
				"xvfb-run -a pnpm exec playwright test --reporter=list", "node_modules/.bin/playwright test", "npx playwright test --update-snapshots",
				"helm unittest helm/superset", "helm unittest -f 'tests/*.yaml' charts/app", "helm unittest --strict --with-subchart=false .", "helm unittest charts/*",
				"docker run --rm -v $(pwd):/apps helmunittest/helm-unittest .", "docker run -ti --rm -v $(pwd):/apps helmunittest/helm-unittest:3.11.1-0.3.0 -o test-output.xml .",
			},
			shouldNotMatch: []string{
				"if test -f go.mod; then echo yes; fi", `test -z "$VAR" && exit 1`,
				"curl https://test.example.com", `echo "testing the build"`,
				"go build ./...", "go vet ./...", "npm run build", "docker build -t app:test .",
				"meson setup _build", "./rerun-tests.shx",
				"cargo nextest archive --archive-file tests.tar.zst", "cargo nextest list",
				"git clone https://github.com/bats-core/bats-core.git", "npm run combats",
				"shellcheck --shell bats test/*.bats", "shfmt --language-dialect bats -d test", `print_info "Running Bats in test"`,
				"php artisan migrate --force", "php artisan route:cache",
				"./gradlew assembleRelease --build-cache", "mvn -Dpackage.name=x compile", "mvn dependency:go-offline", "./gradlew publishToMavenLocal", "./gradlew buildEnvironment",
				"composer validate --strict", "composer run-script lint",
				"latest", "deploy-latest",
				"npx playwright install --with-deps chromium", "npx playwright install-deps", "pnpm exec playwright show-report",
				"npx playwright codegen https://example.com", "npx playwright merge-reports --reporter html ./all-blob-reports",
				"python -m playwright install --with-deps",
				"helm plugin install https://github.com/helm-unittest/helm-unittest --version v1.1.1", "helm lint charts/app", "helm template charts/app",
				"docker pull helmunittest/helm-unittest:3.11.1-0.3.0", "make helm-unittest",
			},
		},
		{
			ruleID: "cypress-rule",
			shouldMatch: []string{
				"npx cypress run", "npx cypress run --browser chrome --record --parallel", "yarn cypress run --component",
				"pnpm exec cypress run --spec cypress/e2e/smoke.cy.ts", "xvfb-run -a npx cypress run",
				"npx start-server-and-test start http://localhost:3000 'cypress run'", "cypress-io/github-action@v6",
			},
			shouldNotMatch: []string{
				"npx cypress open", "npx cypress verify", "npx cypress install", "npx cypress info",
				"npx cypress cache path", "npm ci --prefix ./cypress-tests",
			},
		},
		{
			ruleID:         "syft-rule",
			shouldMatch:    []string{"syft dir:. -o spdx-json", "anchore/sbom-action@v0"},
			shouldNotMatch: []string{"syftly", "mysyft"},
		},
		{
			ruleID:         "cosign-rule",
			shouldMatch:    []string{"cosign sign --yes $IMAGE"},
			shouldNotMatch: []string{"cosigning the release", "cosigner"},
		},
		{
			// A bare "spdx" matches the licence header, which says nothing
			// about whether an SBOM is produced.
			ruleID:      "spdx-tools-rule",
			shouldMatch: []string{"spdx-sbom-generator -p .", "spdx-tools convert"},
			shouldNotMatch: []string{
				"# SPDX-License-Identifier: MIT",
				"SPDX-License-Identifier: Apache-2.0",
				"spdx",
			},
		},
		{
			ruleID:         "slsa-rule",
			shouldMatch:    []string{"slsa-framework/slsa-github-generator", "slsa provenance"},
			shouldNotMatch: []string{"slsawesome", "salsa", "slsax"},
		},
		{
			ruleID:         "github-attestation-rule",
			shouldMatch:    []string{"actions/attest-build-provenance@v1", "actions/attest-sbom@v1"},
			shouldNotMatch: []string{"actions/checkout@v4", "attestation"},
		},
		{
			// trivy writes an SBOM as an output format; "trivy sbom" scans one.
			ruleID:         "trivy-sbom-rule",
			shouldMatch:    []string{"trivy fs --format cyclonedx --output sbom.json .", "trivy image -f spdx-json alpine"},
			shouldNotMatch: []string{"trivy fs .", "trivy image alpine", "trivy config .", "trivy sbom sbom.json", "trivy fs --format sarif ."},
		},
		{
			ruleID:         "cobertura-rule",
			shouldMatch:    []string{"cobertura_splitter --in coverage.xml", "coberturaXML report"},
			shouldNotMatch: []string{"tcobertura"},
		},
		{
			ruleID:         "php-static-analysis-rule",
			shouldMatch:    []string{"vendor/bin/phpstan analyse", "vendor/bin/psalm --shepherd"},
			shouldNotMatch: []string{"psalms.txt"},
		},
		{
			ruleID:         "clang-tidy-rule",
			shouldMatch:    []string{"run-clang-tidy -p build", "clang-tidy src/*.c"},
			shouldNotMatch: []string{"clang -Wall"},
		},
		{
			ruleID:         "shellcheck-rule",
			shouldMatch:    []string{"shellcheck scripts/*.sh", "ludeeus/action-shellcheck"},
			shouldNotMatch: []string{"shellchecker-config"},
		},
		{
			ruleID: "yamllint-rule",
			shouldMatch: []string{
				"yamllint .", "yamllint -c .yamllint.yml config/",
				"ibiqlik/action-yamllint@v3", "ibiqlik/action-yamllint",
				"karancode/yamllint-github-action@v2", "frenck/action-yamllint@v1",
				"uv run yamllint .", "pipx run yamllint .", "python -m yamllint .",
				"poetry run yamllint .", "docker run --rm cytopia/yamllint .",
				"cd charts && yamllint .",
				"pre-commit run yamllint --all-files", "./venv/bin/yamllint .",
				"registry.example/team/yamllint:latest", "registry.example/team/yamllint@sha256:abc",
				"set -e\nyamllint .", "Yamllint", "  yamllint .",
			},
			shouldNotMatch: []string{"yaml-lint-free", "yamllinter"},
		},
		{
			ruleID:         "biome-rule",
			shouldMatch:    []string{"biome check .", "npx @biomejs/biome ci", "pnpm biome lint src", "biomejs/setup-biome"},
			shouldNotMatch: []string{"biome.json", "terraform apply -var biome=forest"},
		},
		{
			ruleID:         "oxlint-rule",
			shouldMatch:    []string{"oxlint", "npx oxlint@latest --deny-warnings", "oxc-project/oxlint-action"},
			shouldNotMatch: []string{"oxlintrc", "toxlint"},
		},
		{
			ruleID:         "next-lint-rule",
			shouldMatch:    []string{"next lint", "next  lint --dir src"},
			shouldNotMatch: []string{"next build", "nextlint"},
		},
		{
			ruleID:         "deno-lint-rule",
			shouldMatch:    []string{"deno lint", "deno lint --unstable"},
			shouldNotMatch: []string{"deno fmt", "deno test"},
		},
		{
			ruleID:         "stylelint-rule",
			shouldMatch:    []string{"stylelint '**/*.css'", "npx stylelint ."},
			shouldNotMatch: []string{"stylelinter"},
		},
		{
			ruleID:         "helm-lint-rule",
			shouldMatch:    []string{"helm lint ./charts/app", "helm lint charts/* --strict", "helm lint . -f values-prod.yaml --with-subcharts"},
			shouldNotMatch: []string{"helm template ./charts/app", "helm install app ./chart --dry-run", "helm package charts/app", "helm repo add bitnami https://charts.bitnami.com/bitnami"},
		},
		{
			ruleID:         "go-vet-rule",
			shouldMatch:    []string{"go vet ./...", "go  vet -tags integration ./..."},
			shouldNotMatch: []string{"go test ./...", "govet", "cargo vet"},
		},
		{
			ruleID:         "staticcheck-rule",
			shouldMatch:    []string{"staticcheck ./...", "dominikh/staticcheck-action"},
			shouldNotMatch: []string{"staticchecker"},
		},
		{
			ruleID:         "kotlin-linter-rule",
			shouldMatch:    []string{"./gradlew ktlintCheck", "ktlint --reporter=plain", "./gradlew detekt"},
			shouldNotMatch: []string{"ktlints", "detektor"},
		},
		{
			ruleID:         "checkstyle-rule",
			shouldMatch:    []string{"./gradlew checkstyleMain checkstyleTest", "mvn checkstyle:check"},
			shouldNotMatch: []string{"checkstyles", "./gradlew build"},
		},
		{
			ruleID:         "swiftlint-rule",
			shouldMatch:    []string{"swiftlint --strict", "norio-nomura/action-swiftlint"},
			shouldNotMatch: []string{"swiftlinter"},
		},
		{
			ruleID:         "dart-analyze-rule",
			shouldMatch:    []string{"flutter analyze", "dart analyze --fatal-infos"},
			shouldNotMatch: []string{"flutter test", "dart analyzer"},
		},
		{
			// Plain tsc is how packages are built, so only --noEmit says
			// "check, don't write". tsc-alias is another tool, and
			// --noEmitOnError is a different flag.
			ruleID: "typescript-noemit-rule",
			shouldMatch: []string{
				"tsc --noEmit", "npx tsc --noEmit", "pnpm exec tsc --noEmit", "bunx tsc --noEmit",
				"bun x tsc --noEmit -p scripts/tsconfig.json", "npx tsc -b --noEmit",
				"npx tsc --noEmit -p convex/tsconfig.json", "./node_modules/.bin/tsc --noEmit",
				"node script/yarn.js tsc -p tsconfig.spec.json --noEmit", "pnpm exec tsc --noEmit --incremental false",
				"vue-tsc --noEmit", "pnpm --filter marktext exec vue-tsc --noEmit -p tsconfig.json",
				"npx tsgo -p packages/twenty-cli/tsconfig.json --noEmit", "tsc --noemit",
			},
			shouldNotMatch: []string{
				"tsc", "pnpm tsc", "tsc -p tsconfig.build.json", "tsc --noEmitOnError", "vue-tsc -b",
				"bun run typecheck:tsc", "tsc-alias -p tsconfig.json --noEmit",
				"uv export --frozen --no-dev --no-emit-project", "npm run typecheck",
			},
		},
		{
			ruleID:         "hadolint-rule",
			shouldMatch:    []string{"hadolint Dockerfile", "hadolint/hadolint-action"},
			shouldNotMatch: []string{"hadolints"},
		},
		{
			ruleID:         "ansible-lint-rule",
			shouldMatch:    []string{"ansible-lint", "ansible-lint playbooks/ --profile production", "ansible/ansible-lint@v25"},
			shouldNotMatch: []string{"ansible-playbook site.yml", "ansible-galaxy install -r requirements.yml", "yamllint ."},
		},
		{
			ruleID: "codespell-rule",
			shouldMatch: []string{
				"codespell",
				`codespell --skip="*.lock,./target" -L crate`,
				"codespell-project/actions-codespell@v2",
				"tox -e codespell",
			},
			shouldNotMatch: []string{"npx cspell lint .", "cp .codespellrc /tmp/", "python -m codespell_lib"},
		},
		{
			ruleID: "chart-testing-rule",
			shouldMatch: []string{
				"ct lint --target-branch ${{ github.event.repository.default_branch }}",
				"ct lint --config ct.yaml --all",
				"ct lint-and-install --chart-dirs charts",
			},
			shouldNotMatch: []string{
				"ct install --target-branch main",
				"ct list-changed --target-branch main",
				"helm/chart-testing-action@v2.8.0",
				"react lint",
			},
		},
		{
			ruleID: "markdownlint-rule",
			shouldMatch: []string{
				`npx markdownlint-cli2 "**/*.md"`, "markdownlint '**/*.md' --ignore node_modules",
				"DavidAnson/markdownlint-cli2-action@v20", "pre-commit run markdownlint --all-files",
				"Lint markdown with markdownlint", "pnpm exec markdownlint-cli2 README.md", "pnpm dlx markdownlint-cli2 README.md",
				"yarn markdownlint-cli2 README.md", "bunx markdownlint-cli2 README.md", "articulate/actions-markdownlint@v1",
			},
			shouldNotMatch: []string{"remark-lint", "markdown-link-check README.md", "mymarkdownlint", "markdownlinter"},
		},
		{
			ruleID: "tflint-rule",
			shouldMatch: []string{"tflint --recursive", "tflint --init && tflint -f compact",
				"tflint", "tflint --chdir=infra --var-file=prod.tfvars", "ghcr.io/terraform-linters/tflint:v0.55.0"},
			shouldNotMatch: []string{"terraform fmt -check", "terraform validate", "tflint_plugins"},
		},
		{
			ruleID:         "credo-rule",
			shouldMatch:    []string{"mix credo --strict"},
			shouldNotMatch: []string{"credo", "mix test"},
		},
		{
			ruleID:         "standardrb-rule",
			shouldMatch:    []string{"bundle exec standardrb"},
			shouldNotMatch: []string{"standard"},
		},
		{
			ruleID:         "haskell-lua-linter-rule",
			shouldMatch:    []string{"hlint src", "luacheck ."},
			shouldNotMatch: []string{"hlinter"},
		},
		{
			ruleID:         "vite-plus-lint-rule",
			shouldMatch:    []string{"vp lint --deny-warnings", "pnpm exec vp  lint"},
			shouldNotMatch: []string{"vp build", "mvp lint"},
		},
		{
			ruleID:         "warp-rule",
			shouldMatch:    []string{"warp mixed --host localhost:9000 --duration 5m", "curl -fsSL https://github.com/minio/warp/releases/download/v1.1.4/warp_Linux_x86_64.tar.gz", "docker run minio/warp get --obj.size 1MiB"},
			shouldNotMatch: []string{"cargo build -p warp", "use warp::Filter", "time warp drive", "minio/warpspeed"},
		},
		{
			ruleID:         "clippy-rule",
			shouldMatch:    []string{"cargo clippy --all-targets -- -D warnings", "actions-rs/clippy-check"},
			shouldNotMatch: []string{"cargo build", "clippy.toml"},
		},
		{
			ruleID:         "scan-build-rule",
			shouldMatch:    []string{"scan-build -o report ninja", "scan-build --status-bugs make"},
			shouldNotMatch: []string{"build-scan"},
		},
		{
			ruleID:         "reuse-rule",
			shouldMatch:    []string{"reuse lint", "fsfe/reuse-action"},
			shouldNotMatch: []string{"reuse download --all"},
		},
		{
			ruleID: "license-checker-rule",
			shouldMatch: []string{
				"npx license-checker --production --onlyAllow \"MIT;ISC;Apache-2.0\"",
				"license-checker --failOn \"GPL-3.0;AGPL-3.0\"",
				"npx license-checker-rseidelsohn --onlyAllow MIT",
				"npx license-checker --json --out licenses.json",
				"npx license-checker@4 --onlyAllow MIT",
			},
			shouldNotMatch: []string{"addlicense -check .", "licensee detect .", "npm run check-licenses", "pip-license-checker --fail-on GPL", "npx license-checker-webpack-plugin"},
		},
		{
			ruleID:         "cargo-deny-licenses-rule",
			shouldMatch:    []string{"cargo deny check", "cargo deny --all-features check licenses", "cargo-deny check licenses bans", "EmbarkStudios/cargo-deny-action"},
			shouldNotMatch: []string{"cargo deny init"},
		},
		{
			ruleID:         "cargo-deny-advisories-rule",
			shouldMatch:    []string{"cargo deny check", "cargo deny check advisories", "EmbarkStudios/cargo-deny-action"},
			shouldNotMatch: []string{"cargo deny fetch"},
		},
		{
			ruleID:         "dependency-review-rule",
			shouldMatch:    []string{"actions/dependency-review-action"},
			shouldNotMatch: []string{"actions/dependency-review-action-fork", "my/actions/dependency-review-action"},
		},
		{
			ruleID:         "audit-ci-rule",
			shouldMatch:    []string{"npx audit-ci --high", "npx audit-ci@^7 --config audit-ci.jsonc", "yarn audit-ci --moderate", "pnpm dlx audit-ci@^7 --critical --report-type full", "audit-ci"},
			shouldNotMatch: []string{"npm ci", "audit-cirrus", "git add audit-ci.jsonc", "npm audit --audit-level=high"},
		},
		{
			ruleID:         "codeql-actions-rule",
			shouldMatch:    []string{"codeql database create --language=actions", "codeql database create --language=go,actions,python"},
			shouldNotMatch: []string{"codeql database create --language=go", "codeql database create", "codeql database create --language=github-actions"},
		},
		{
			ruleID:         "trivy-secret-rule",
			shouldMatch:    []string{"trivy fs --scanners secret .", "trivy repo --scanners vuln,secret ."},
			shouldNotMatch: []string{"trivy fs .", "trivy fs --scanners vuln ."},
		},
		{
			ruleID:         "buf-lint-rule",
			shouldMatch:    []string{"buf lint", "buf lint proto/ --error-format=github-actions", "bufbuild/buf-action@v1", "bufbuild/buf-lint-action@v1"},
			shouldNotMatch: []string{"buf generate", "buf push", "buf format -w", "buf config ls-lint-rules", "bufbuild/buf-setup-action@v1"},
		},
		{
			ruleID: "phpcs-rule",
			shouldMatch: []string{
				"phpcs", "phpcs --standard=PSR12 src/", "vendor/bin/phpcs -p", "./vendor/bin/phpcs",
				"composer exec phpcs -- --report=checkstyle", "docker run cytopia/phpcs:latest",
			},
			shouldNotMatch: []string{"phpcbf src/", "cp phpcs.xml.dist phpcs.xml", "php-cs-fixer fix --dry-run"},
		},
		{
			ruleID: "formatter-check-rule",
			shouldMatch: []string{
				"cargo fmt --all -- --check", "cargo fmt --check", "cargo fmt -- --config imports_granularity=Item --check", "rustfmt --check src/main.rs",
				"prettier --check .", "prettier@3.9.0 --check .", `prettier "docs/**/*.md" --check`, "prettier -c src", "prettier --list-different .",
				"black --check .", "black . --check", "black --check --diff src", "isort --check .", "isort --check-only --diff src",
				"dotnet format --verify-no-changes --verbosity minimal", "dart format --output=none --set-exit-if-changed lib test",
				"terraform fmt -check -recursive", "tofu fmt -recursive -check -diff", "shfmt -i 4 -ci -sr -d scripts", "shfmt --diff .",
				"clang-format --dry-run --Werror src/a.c", "clang-format-18 -n --Werror src/a.c", "clang-format --Werror --dry-run a.c",
				"mvn spotless:check", "./gradlew spotlessCheck", "dprint check", "oxfmt --check .", "mix format --check-formatted", "deno fmt --check",
				"dprint/check", "DoozyX/clang-format-lint-action",
			},
			shouldNotMatch: []string{
				"cargo fmt --all", "cargo fmt", "rustup component add rustfmt", "prettier --write .", "black .", "isort .",
				"dotnet format", "dart format lib", "terraform fmt -recursive", "shfmt -w .", "shfmt -l .", "clang-format -i src/a.c",
				"clang-format --Werror -i src/a.c", "mvn spotless:apply", "dprint fmt", "oxfmt .", "mix format", "deno fmt",
				"gofmt -l .", "ruff check --select black", "echo blacklist --check", "dprint/check-extra",
			},
		},
		{
			ruleID: "pre-commit-rule",
			shouldMatch: []string{
				"pre-commit run --all-files", "pre-commit run --show-diff-on-failure --color=always --all-files", "uvx pre-commit run -a",
				"prek run --all-files", "prek", "prek --all-files --show-diff-on-failure", `prek "${prek_args[@]}" --all-files`, "make lint\nprek run",
				"pre-commit/action", "j178/prek-action",
			},
			shouldNotMatch: []string{
				"pre-commit install", "pre-commit install --install-hooks", "pre-commit autoupdate", "pre-commit gc",
				"prek install", "prek prepare-hooks", "echo prek is fast", "pre-commit/action-extra", "./.github/actions/install-prek",
			},
		},
		{
			ruleID: "os-code-signing-rule",
			shouldMatch: []string{
				`codesign --verbose -f -s "$MACOS_CERTIFICATE_NAME" -o runtime App.app`, `codesign --sign "Developer ID Application: X" --timestamp app`,
				`xcrun notarytool submit "App.dmg" --keychain-profile "p" --wait`, "notarytool submit app.zip --wait",
				"signtool sign /fd sha256 /tr http://ts app.exe", "signtool.exe sign /a app.exe", "osslsigncode sign -pkcs12 c.p12 -in a.exe -out b.exe",
				"rcodesign sign target/release/deno --code-signature-flags=runtime",
			},
			shouldNotMatch: []string{
				"codesign -s - App.app", "codesign --sign - --force App.app", "codesign --verify --deep App.app", "codesign -dv App.app",
				"security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k pass build.keychain",
				`xcrun notarytool store-credentials "profile" --key k.p8`, "xcrun notarytool log id", "signtool verify /pa app.exe",
				"rcodesign print-signature-info app.dmg", `echo "codesign -s identity"`,
			},
		},
		{
			ruleID:         "azure-artifact-signing-rule",
			shouldMatch:    []string{"azure/trusted-signing-action", "Azure/artifact-signing-action"},
			shouldNotMatch: []string{"azure/login", "azure/artifact-signing-action-extra"},
		},
		{
			ruleID:         "signpath-rule",
			shouldMatch:    []string{"signpath/github-action-submit-signing-request"},
			shouldNotMatch: []string{"./.github/actions/install-signpath-module"},
		},
		{
			ruleID:         "esigner-rule",
			shouldMatch:    []string{"sslcom/esigner-codesign"},
			shouldNotMatch: []string{"apple-actions/import-codesign-certs"},
		},
		{
			ruleID:         "cifuzz-rule",
			shouldMatch:    []string{"google/oss-fuzz/infra/cifuzz/actions/run_fuzzers"},
			shouldNotMatch: []string{"google/oss-fuzz/infra/cifuzz/actions/build_fuzzers"},
		},
		{
			ruleID:         "clusterfuzzlite-rule",
			shouldMatch:    []string{"google/clusterfuzzlite/actions/run_fuzzers"},
			shouldNotMatch: []string{"google/clusterfuzzlite/actions/build_fuzzers"},
		},
		{
			ruleID:         "cargo-fuzz-rule",
			shouldMatch:    []string{"cargo fuzz run parser -- -max_total_time=60", "cargo +nightly fuzz run fuzz_target_1"},
			shouldNotMatch: []string{"cargo fuzz build -s none", "cargo install cargo-fuzz", "cargo fuzz list"},
		},
		{
			ruleID:         "github-attestation-rule",
			shouldMatch:    []string{"actions/attest", "actions/attest-build-provenance", "actions/attest-sbom"},
			shouldNotMatch: []string{"actions/attest-foo", "actions/attestation-viewer"},
		},
		{
			ruleID: "test-results-step-rule",
			shouldMatch: []string{
				"PublishTestResults", "EnricoMi/publish-unit-test-result-action", "EnricoMi/publish-unit-test-result-action/linux",
				"dorny/test-reporter", "mikepenz/action-junit-report", "test-summary/action",
			},
			shouldNotMatch: []string{"dorny/paths-filter", "actions/upload-artifact"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.ruleID, func(t *testing.T) {
			rule := findRule(t, rs, tt.ruleID)
			for _, s := range tt.shouldMatch {
				assert.True(t, rule.Regex.MatchString(s), "%s should match %q", tt.ruleID, s)
				if rule.Literals != nil {
					assert.True(t, slices.ContainsFunc(rule.Literals, func(literal string) bool { return strings.Contains(strings.ToLower(s), literal) }),
						"%s's literals %q would skip %q, which it matches", tt.ruleID, rule.Literals, s)
				}
				if rule.Unless != nil {
					assert.False(t, rule.Unless.MatchString(s), "%s should not except %q", tt.ruleID, s)
				}
			}
			for _, s := range tt.shouldNotMatch {
				assert.False(t, rule.Regex.MatchString(s), "%s should NOT match %q", tt.ruleID, s)
			}
		})
	}
}

// A command a rule's regex matches, but that does not do what the rule credits.
func TestRuleUnlessExceptsLookalikeCommands(t *testing.T) {
	rs, err := rules.NewRulesService()
	assert.NoError(t, err)

	for id, excepted := range map[string][]string{
		"bundler-audit-rule":         {"bundle-audit update", "bundle exec bundle-audit update", "bundle audit version", "rake bundle:audit:update"},
		"cargo-deny-licenses-rule":   {"cargo deny check advisories", "cargo deny check bans sources"},
		"cargo-deny-advisories-rule": {"cargo deny check licenses", "cargo deny check bans licenses"},
		"codeql-rule":                {"codeql database create --language=actions"},
		"zizmor-rule":                {"zizmorcore/zizmor-action"},
		"npm-audit-rule":             {"npm audit signatures", "npm audit signatures --omit=dev"},
		"tflint-rule":                {"tflint --init", "tflint -v"},
		"snyk-agent-scan-rule":       {"uvx snyk-agent-scan@latest --ci --no-skills ~/.vscode/mcp.json"},
		"trivy-rule":                 {"trivy fs --scanners secret .", "trivy repo --scanners misconfig,secret .", "trivy fs --security-checks config .", "trivy image --format cyclonedx alpine", "trivy fs -f spdx ."},
		"unit-test-command-rule": {
			"mvn package -Dmaven.test.skip=true", "./mvnw -B install -DskipTests test", "gradle build -x test", "mvn -B clean install -DskipTests",
			"./gradlew check --exclude-task test", "cargo test --no-run", "cargo test --all --no-run --locked",
			"cargo nextest run --no-run", "cargo nextest run --workspace --no-run --locked",
			"tox -e lint", "tox -e mypy,docs", "tox --envlist=pre-commit",
			"npx playwright test --list", "pnpm exec playwright test --config tests/playwright.config.ts --list --reporter=json",
			"helm unittest --help", "helm unittest --help >/dev/null",
		},
		"unit-test-rule": {"deploy-test", "publish-test-pypi", "report-test-failure", "tests-result", "set-test-package-matrix", "Snyk Test", "Test GitHub OIDC"},
		"taurus-rule":    {"pip install bzt", "pip3 install bzt", "bzt --help", "bzt -h"},
	} {
		rule := findRule(t, rs, id)
		for _, command := range excepted {
			assert.True(t, rule.Regex.MatchString(command), "%s should match %q before excepting it", id, command)
			assert.True(t, rule.Unless.MatchString(command), "%s should except %q", id, command)
		}
	}
}
