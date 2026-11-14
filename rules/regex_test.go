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
			ruleID: "sobelow-rule",
			shouldMatch: []string{
				"mix sobelow --exit", "mix sobelow --exit medium --skip", "mix sobelow --config",
				"mix sobelow", "sobelow/action", "sobelow",
				"~/.mix/escripts/sobelow --exit low",
			},
			shouldNotMatch: []string{"mix credo", "mix deps.audit", "mix test", "my-sobelow-wrapper", "mysobelow"},
		},
		{
			ruleID: "kingfisher-rule",
			shouldMatch: []string{
				"kingfisher scan .", "kingfisher scan --git-history full .",
				"docker run ghcr.io/mongodb/kingfisher scan /src",
				"docker run ghcr.io/mongodb/kingfisher:v1.0 scan /src",
				"./bin/kingfisher scan ./src",
			},
			shouldNotMatch: []string{
				"brew install kingfisher", "kingfisher --version", "kingfishers",
				"kingfisher scan-results", "mykingfisher scan .",
			},
		},
		{
			ruleID: "cargo-vet-rule",
			shouldMatch: []string{
				"cargo vet", "cargo vet check", "cargo vet --locked",
				"cargo +stable vet check", "cd workspace && cargo vet check --locked",
				"cargo vet --locked check", "cargo vet --frozen --locked",
				"cargo vet --output-format json",
			},
			shouldNotMatch: []string{
				"cargo install cargo-vet", "cargo-vet check", "mycargo vet check",
				"cargo vendor", "cargo verify-project",
			},
		},
		{
			ruleID: "opengrep-rule",
			shouldMatch: []string{
				"opengrep scan --config auto --error", "opengrep scan .",
				"platform-sec/opengrep-action", "opengrep ci", "opengrep --experimental ci",
				"opengrep --config auto .", "./bin/opengrep scan --config auto .",
				"docker run opengrep/opengrep scan -f rules src",
				"scripts/run-opengrep.sh --changed --sarif --error",
			},
			shouldNotMatch: []string{
				"grep -r foo .", "semgrep scan", "myopengrep scan .",
			},
		},
		{
			ruleID: "kube-linter-rule",
			shouldMatch: []string{
				"kube-linter lint k8s/",
				"kube-linter lint --config .kube-linter.yaml charts/",
				"./bin/kube-linter lint manifests/",
				"kube-linter --config .kube-linter.yaml lint k8s/",
				"docker run --rm -v $PWD:/w stackrox/kube-linter:v0.7.2 lint /w/k8s",
				"bash -c \"kube-linter lint k8s/\"",
				"stackrox/kube-linter-action@v1",
			},
			shouldNotMatch: []string{
				"kubectl apply -f k8s/", "kubeval k8s/", "kube-linter --version",
				"my-kube-linter lint k8s/", "stackrox/kube-linter-action-extra@v1",
				"kube-linter --config .kube-linter.yaml --version",
				"my-kube-linter:v0.7.2 lint k8s/",
				"docker run --rm stackrox/kube-linter:v0.7.2 --help",
			},
		},
		{
			ruleID: "pinact-rule",
			shouldMatch: []string{
				"pinact run --check",
				"pinact run --diff",
				"pinact run --fix=false --no-api",
				"aqua exec -- pinact run --check",
				"pinact run",
				"./bin/pinact run --fix false",
			},
			shouldNotMatch: []string{
				"pinact init", "pinact migrate", "./scripts/pin-actions.sh",
				"pinact runn --check", "my-pinact run --check",
			},
		},
		{
			ruleID: "secretlint-rule",
			shouldMatch: []string{
				`npx secretlint "**/*"`,
				"secretlint --format sarif .",
				`secretlint --format @secretlint/secretlint-formatter-sarif "**/*"`,
				`docker run secretlint/secretlint secretlint "**/*"`,
			},
			shouldNotMatch: []string{"secretlinter", "my-secretlint", "secretlint-wrapper", "mysecretlint"},
		},
		{
			ruleID: "talisman-rule",
			shouldMatch: []string{
				"talisman --scan",
				"talisman --scan --ignoreHistory",
				"./bin/talisman --scan --reportdirectory=reports",
				"./talisman -i . --scan",
				"carhartl/talisman-secrets-scan-action",
				"digitalservicebund/talisman-secrets-scan-action",
			},
			shouldNotMatch: []string{
				"talismans --scan", "talisman-installer", "my-talisman --scan", "talisman_scanner",
				"flask-talisman", "talisman-secrets-scan-action-helper",
			},
		},
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
			ruleID: "blastradius-rule",
			shouldMatch: []string{
				"jondot/blastradius", "br hunt npm:evil-dep@1.2.3 --path . --fail-if found",
				"bin/br hunt npm:evil-dep@1.2.3 --path tests/action/bad --format sarif",
				"npx -y @jondot/blastradius hunt GHSA-xxxx-xxxx-xxxx --path .",
				"npx -y @jondot/blastradius@0.1.0 hunt npm:x@1.0.0", "cargo build\nbr hunt sha256:00 --path .",
			},
			shouldNotMatch: []string{
				"jondot/blastradius-extra", "br init --org acme", "br index", "br demo", "br hosts add ci --kind runner",
				"npx -y @jondot/blastradius demo", "git br hunt", "brhunt", "blastradius hunt", "abbr hunt",
			},
		},
		{
			ruleID:         "blastradius-precommit-rule",
			shouldMatch:    []string{"blastradius", "pre-commit run --all-files\nblastradius", "blastradius package-lock.json"},
			shouldNotMatch: []string{"blastradius-scan", "echo blastradius", "./blastradius.sh", "jondot/blastradius"},
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
				"basedpyright", "uv run --no-sync basedpyright",
				"uv run --no-sync basedpyright tests/e2e tests/e2e_harness",
				"uv run --no-sync basedpyright --pythonpath .venv/bin/python --project tests/typing/pyrightconfig.json",
				".venv/bin/basedpyright --level error", "basedpyright --gitlabcodequality report.json",
				"uvx basedpyright@1.40.2", "npx basedpyright",
				"pyrefly check",
				"pyrefly check --python-version=3.8 --preset=basic --check-unannotated-defs --summary",
				"uv run pyrefly check src/", "uv run --project . --dev pyrefly check --summary=none",
				"uvx pyrefly@1.4.1 check", "pyrefly check --output-format=github",
				"facebook/pyrefly", "meta/pyrefly",
			},
			shouldNotMatch: []string{
				"mypyc-build", "pyrightish",
				"ty version", "ty server", "ty explain rule unresolved-import", "uv run ty", "ty checkout",
				"pretty check", "./scripts/ty-check.sh", "cargo build --profile=profiling --bin=ty",
				"mv target/profiling/ty ty-new",
				"git diff --exit-code .basedpyright/baseline.json", "basedpyright-langserver --stdio",
				"pyrefly init", "pyrefly lsp", "pyrefly dump-config", "pyrefly suppress",
				"pyrefly coverage report", "uv run --directory api --dev pyrefly report",
				"pyrefly coverage check --public-only src/", "./dev/pyrefly-check-local \"${targets[@]}\"",
				"LINTER_GROUP=pyrefly .github/scripts/lintrunner.sh",
				"diff -u /tmp/pyrefly_base.txt /tmp/pyrefly_pr.txt > pyrefly_diff.txt",
				"pyrefly checkout",
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
			ruleID: "ghalint-rule",
			shouldMatch: []string{
				"ghalint run",
				"ghalint run-action",
				"ghalint act",
				"aqua exec -- ghalint run",
			},
			shouldNotMatch: []string{
				"aqua g -i suzuki-shunsuke/ghalint",
				"ghalints",
			},
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
			// HTMLHint's name appears in config, matcher, report and script names;
			// only the command that runs the linter counts.
			ruleID: "htmlhint-rule",
			shouldMatch: []string{
				"htmlhint", `htmlhint "**/*.html" --format unix`,
				`npx htmlhint "src" --config .htmlhintrc`,
				`npx --yes htmlhint "$file" --config .htmlhintrc.json`,
				`npx --yes htmlhint@1 "index.html"`, `npx htmlhint . --format sarif`,
				`pnpm exec htmlhint "**/*.html"`, `pnpm dlx htmlhint .`,
				`yarn htmlhint src`, `yarn exec htmlhint .`, `yarn dlx htmlhint .`,
				`bunx htmlhint .`, `bun x htmlhint .`, `pnpm htmlhint .`,
				`npm exec -- htmlhint .`, `./node_modules/.bin/htmlhint index.html`,
				"lint\nhtmlhint index.html",
			},
			shouldNotMatch: []string{
				"cp .htmlhintrc /tmp/",
				`echo "::add-matcher::.github/workflows/htmlhint-problem-matcher.json"`,
				`OUT="${GITHUB_WORKSPACE}/quality/analysis/raw/htmlhint.json"`,
				"test -s htmlhint.json", "cat htmlhint.sarif", "npm run htmlhint-sarif",
				"Run HTMLHint (SARIF)", "gulp-htmlhint", "htmlhint-loader",
				"echo htmlhint .", "not-htmlhint index.html",
			},
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
			ruleID: "ripsecrets-rule",
			shouldMatch: []string{
				"ripsecrets", "ripsecrets src/ config/",
				"ripsecrets --strict-ignore $(git diff --cached --name-only)",
				"/usr/local/bin/ripsecrets", "sirwart/ripsecrets@v0.1.8",
			},
			shouldNotMatch: []string{
				"rip secrets", "./ripsecrets-report.py", "grep -r secrets .",
				"ripsecrets-helper", "myripsecrets",
			},
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
			ruleID: "npm-provenance-rule",
			shouldMatch: []string{"npm publish --provenance --access public", "pnpm -r publish --provenance", "yarn npm publish --provenance",
				"npm publish --provenance=true", "NPM_CONFIG_PROVENANCE=true", "npm_config_provenance=1", "CI=true\nNPM_CONFIG_PROVENANCE='true'"},
			shouldNotMatch: []string{"npm publish", "npm publish --provenance=false", "NPM_CONFIG_PROVENANCE=false", "MY_NPM_CONFIG_PROVENANCE=true",
				"actions/attest-build-provenance", "npm view pkg --provenance"},
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
			shouldMatch:    []string{"SonarSource/sonarqube-scan-action", "sonar-scanner -Dsonar.host.url=x", "sonarcloud scan", "npx sonarqube-scanner", "./gradlew sonarqube", "Run SonarCloud"},
			shouldNotMatch: []string{"sonar", "sonarian", "build-sonar-module", "gcovr -r . --sonarqube coverage.xml", "jest --coverage --coverageReporters=lcov,sonarqube", "vitest --coverage.reporter=sonarqube"},
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
			ruleID: "polaris-rule",
			shouldMatch: []string{
				"polaris audit --audit-path deploy/",
				"./bin/polaris audit --set-exit-code-on-danger",
				"docker run --rm us-docker.pkg.dev/fairwinds-ops/oss/polaris:v10.2.0 audit --audit-path .",
				"polaris -c config.yaml audit --set-exit-code-below-score 90",
			},
			shouldNotMatch: []string{
				"polaris dashboard", "polaris fix --files-path deploy/",
				"polaris --version", "shopify-polaris audit",
				"my-polaris audit", "polaris audit-report", "curl -O polaris",
			},
		},
		{
			ruleID:         "kube-score-rule",
			shouldMatch:    []string{"kube-score score deploy/*.yaml", "helm template . | kube-score score -", "kube-score score --exit-one-on-warning k8s/", "docker run -v $PWD:/project zegl/kube-score score /project/k8s/*.yaml", "kube-score score"},
			shouldNotMatch: []string{"kubectl score", "kube-scorecard", "kube-score version", "kube-score help score", "kube-score completion bash", "kube-scorecard score k8s/"},
		},
		{
			ruleID: "kubeconform-rule",
			shouldMatch: []string{
				"kubeconform -strict -summary -",
				"kubeconform -strict -summary -kubernetes-version 1.30.0 rendered.yaml",
				"./kubeconform -strict -summary -kubernetes-version 1.35.0 weather-rendered deploy/nvidia-device-plugin.yaml",
				"/kubeconform -summary -output json kubeconfigs/",
				`docker run --rm -v "$PWD":/manifests ghcr.io/yannh/kubeconform:latest -strict -ignore-missing-schemas -summary /manifests/rendered.yaml`,
				`docker run --rm -v "$PWD:/deploy:ro" -w /deploy ghcr.io/yannh/kubeconform:v0.8.0@sha256:faffaf43f95aa6425306e1ab8d6fcad72acb9049158f38e574c085ea1ec0f64e -strict -summary ./*.yaml`,
				"docker://ghcr.io/yannh/kubeconform",
				"docker://ghcr.io/yannh/kubeconform:latest",
				"ghcr.io/yannh/kubeconform:latest-alpine",
				"helm kubeconform --summary .",
				"kubeconform",
			},
			shouldNotMatch: []string{
				"grep ' kubeconform-linux-amd64.tar.gz$' kubeconform-CHECKSUMS",
				"python scripts/security_tools.py actionlint gitleaks trivy kubeconform",
				"https://github.com/yannh/kubeconform/releases/download/v0.8.0/kubeconform-linux-amd64.tar.gz",
				"helm plugin install https://github.com/jtyr/kubeconform-helm",
				"kube-score score k8s/",
				"kubescape scan k8s/",
				"kube-linter lint k8s/",
				"kubectl apply --dry-run=server -f k8s/",
				"hl-infra/kubeconform-runner",
			},
		},
		{
			ruleID: "kubeval-rule",
			shouldMatch: []string{
				"kubeval --strict -d k8s/",
				"kubeval -v 1.20.0 manifests/*.yaml",
				"helm kubeval charts/app",
				"docker run -v $PWD/fixtures:/fixtures garethr/kubeval fixtures/*",
				"garethr/kubeval:latest",
				"instrumenta/kubeval-action",
			},
			shouldNotMatch: []string{
				"helm plugin install https://github.com/instrumenta/helm-kubeval",
				"kube-score score k8s/",
				"kubescape scan k8s/",
				"kube-linter lint k8s/",
				"kubectl apply --dry-run=server -f k8s/",
				"kubevalidator check",
			},
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
				"gotestsum -- ./...", "gotestsum --format github-actions ./...",
				"gotestsum --format github-actions -- -race ./...",
				"gotestsum --junitfile results.xml --packages ./integration -- -v",
				"./bin/gotestsum --format testname",
				"go run gotest.tools/gotestsum@v1.13.0 --rerun-fails=2 --packages=./... -- -v",
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
				`docker cp "$CTN:c:\gopath\bin\gotestsum.exe" bin\`, "go build -o bin/gotestsum gotest.tools/gotestsum",
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
			// "knip" is also a script name, config file and cache path, so only
			// a command that runs the program counts.
			ruleID: "knip-rule",
			shouldMatch: []string{
				"knip", "knip --production", "knip --no-config-hints",
				"knip --config ../scripts/knip.config.ts",
				"knip --exclude exports,nsExports,types,nsTypes,enumMembers,classMembers,duplicates",
				"knip-bun", "./node_modules/.bin/knip --no-exit-code --reporter json",
				"npx knip", "npx --yes knip@5 --no-progress --include files,dependencies,unlisted",
				"bunx knip", "pnpm knip --no-progress --reporter markdown",
				"pnpm knip --workspace apps/studio --no-progress --reporter symbols --reporter github-actions",
				"pnpm run knip --production --exclude exports,types", "pnpm exec knip",
				"pnpm dlx knip@6", "yarn knip --reporter github-actions",
				"bun run knip --no-exit-code", "npm exec -- knip",
			},
			shouldNotMatch: []string{
				"npm run knip:ci", "pnpm knip:check", "pnpm lint:knip:production",
				"npx nx run-many -t knip --parallel=1", "rm -rf node_modules/.cache/knip",
				"test -f config/knip.config.ts", "cp knip.json /tmp/",
				"knip_json_exit_code=$?", "npm init @knip/config", "pnpm update knip",
				"Studio Dead Code (knip)",
			},
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
			ruleID: "typos-rule",
			shouldMatch: []string{
				"typos",
				"typos --config=./.config/typos.toml",
				"typos --format brief",
				"./bin/typos .",
				"uvx typos",
				"uvx typos@1.29.0 .",
				"uv run --only-group check typos",
				"nix develop -c typos",
				"pre-commit run typos --all-files",
				"crate-ci/typos@v1.48.0",
				"crate-ci/typos@master",
				"run: typos",
			},
			shouldNotMatch: []string{
				".github/scripts/typos.sh", "./scripts/check-typos.sh", "typos-lsp",
				"cargo binstall typos-cli", "cp _typos.toml /tmp/", "git commit -m fix-typos",
				"codespell --skip=typos.toml", "cspell lint --report=typos .",
			},
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
			ruleID: "cspell-rule",
			shouldMatch: []string{
				"cspell",
				`cspell --config ./.cspell/cspell.json "**/*.md"`,
				`npx cspell "**/*"`,
				"npx cspell lint .",
				"npx --yes cspell@9 lint --no-progress .",
				"pnpm cspell -c ./rfd/cspell.json rfd",
				`pnpm exec cspell "docs/**/*.md" --no-progress`,
				"yarn cspell --show-context 'src/**/*.js'",
				"bunx cspell .",
				"npm exec -- cspell .",
				"./node_modules/.bin/cspell .",
				"cspell-cli lint .",
				"streetsidesoftware/cspell-action@v9",
			},
			shouldNotMatch: []string{
				"node scripts/clean-cspell.js",
				"yarn upgrade cspell@^9 --ignore-engines",
				"cp cspell.json /tmp/",
				"rm -rf .cspellcache",
				"@cspell/dict-en_us",
				"cspell:software-terms/software-terms.txt",
				"codespell",
				"Cleanup cspell.json",
			},
		},
		{
			ruleID: "vulture-rule",
			shouldMatch: []string{
				"vulture",
				"vulture src tests --min-confidence 80",
				"vulture vulture/ tests/",
				"vulture --min-confidence 100 --exclude _vendor .",
				`uv run vulture --exclude "contrib,tests,conftest.py,.venv" --min-confidence 100 .`,
				"uv run vulture app/ vulture_whitelist.py --min-confidence 80",
				"uvx vulture src tests .vulture_white_list.py",
				"uvx vulture@2.16 src",
				"poetry run vulture . --exclude=venv",
				"python -m vulture --min-confidence 80 congressionalrecord",
				"python3 -m vulture src",
				"xargs uvx vulture --min-confidence 80",
				".venv/bin/vulture src",
				"vulture mydir --make-whitelist",
				"pre-commit run vulture --all-files",
				"gtkacz/vulture-action",
				"gtkacz/vulture-action@1.0.0",
			},
			shouldNotMatch: []string{
				`section "vulture: $pkg"`,
				"python vulture_whitelist.py",
				"python scripts/run_vulture.py",
				"pytest tests/test_vulture.py",
				"grep -c . vulture-report.txt",
				"deadcode .",
				"pylint --enable=unused-import src",
				"gtkacz/vulture-action-extra",
			},
		},
		{
			ruleID: "cargo-machete-rule",
			shouldMatch: []string{
				"cargo machete",
				"cargo machete --skip-target-dir",
				"cargo machete --with-metadata",
				"cargo machete --fix",
				"cargo machete --skip tests --skip examples",
				"cargo machete crates/",
				"cargo +stable machete",
				"cargo-machete",
				"cargo-machete --skip-target-dir .",
				"~/.cargo/bin/cargo-machete --json",
				"bnjbvr/cargo-machete",
				"docker run --rm -v $(pwd):/src ghcr.io/bnjbvr/cargo-machete:latest",
				"docker://ghcr.io/bnjbvr/cargo-machete",
				"ghcr.io/bnjbvr/cargo-machete:latest",
			},
			shouldNotMatch: []string{
				"grep -A3 'package.metadata.cargo-machete' Cargo.toml",
				"actionlint .github/workflows/cargo-machete.yml",
				"sha256sum -c cargo-machete-v0.9.2-x86_64-unknown-linux-musl.tar.gz.sha256",
				"cargo machete-v0.9.2",
				// Guard against future changes making machete match cargo-shear (#620)
				"cargo shear",
				"cargo clippy --all-targets",
			},
		},
		{
			ruleID: "cargo-udeps-rule",
			shouldMatch: []string{
				"cargo udeps",
				"cargo udeps --locked --all-targets",
				"cargo udeps --manifest-path ${{ matrix.path }}/Cargo.toml --all-targets --all-features",
				"cargo +nightly udeps",
				"cargo +nightly-2025-06-01 udeps --workspace --output json",
				"cargo-udeps udeps --all-targets",
				"udeps",
				// Multiline cases: ^ should match after newline
				"cargo build\ncargo-udeps udeps --all-targets",
				"pre-commit run --all-files\nudeps",
			},
			shouldNotMatch: []string{
				"grep -n 'workspace.metadata.cargo-udeps' Cargo.toml",
				"sha256sum cargo-udeps-v0.1.61-x86_64-unknown-linux-gnu.tar.gz",
				"Udeps",
				"Create udeps matrix",
				"cargo shear",
				"cargo clippy --all-targets",
			},
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
			ruleID:         "size-limit-rule",
			shouldMatch:    []string{"npx size-limit --json", "pnpm exec size-limit", "yarn size-limit", "bun x size-limit", "npx -y size-limit@11", "./node_modules/.bin/size-limit", "andresz1/size-limit-action@v1", "ant-design/size-limit-action@master", "npx size-limit --why", "yarn size-limit --why --hide-passed", "size-limit", "npx size-limit", "pnpm exec size-limit --json", "pnpm size-limit", "bunx size-limit", "npm exec -- size-limit", "./node_modules/.bin/size-limit --hide-passed", `npx size-limit --limit "10 kB" dist/bundle.js`, "andresz1/size-limit-action", "ant-design/size-limit-action"},
			shouldNotMatch: []string{"/size-limit-override", "@size-limit/preset-app", "size-limit-preset-node-lib", ".size-limit.json", "./dev-packages/size-limit-gh-action", "PR Size Limit", "echo size-limit", `gh pr comment 42 --body "/size-limit-override"`, `jq '.[].limit' .size-limit.json`, "@size-limit/file", "pr-size-limit-42", "npm run size"},
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
			ruleID: "lychee-rule",
			shouldMatch: []string{
				"lychee .", "lychee --no-progress --config lychee.toml './**/*.md'", "lychee --scheme https --timeout 60 --exclude-all-private .",
				"./lychee --offline docs/", "docker run --rm -v $PWD:/input lycheeverse/lychee:latest /input/README.md",
				"lycheeverse/lychee-action@v2", "lycheeverse/lychee-action@e7477775783ea5526144ba13e8db5eec57747ce8",
			},
			shouldNotMatch: []string{
				"cat lychee/out.md", "gh issue create --body-file ./lychee/out.md", "cp lychee.toml /tmp/", "rm -rf .lycheecache",
				"tar -xzf lychee-x86_64-unknown-linux-gnu.tar.gz", "markdown-link-check README.md",
				"key: cache-lychee-${{ github.sha }}",
			},
		},
		{
			ruleID: "markdown-link-check-rule",
			shouldMatch: []string{
				"markdown-link-check",
				"markdown-link-check ./docs",
				"markdown-link-check --reporters junit README.md",
				"npx markdown-link-check -c .mlc.json README.md",
				"npx --yes markdown-link-check@3.15.0 README.md",
				`find . -name \*.md -print0 | xargs -0 -n1 markdown-link-check`,
				"xargs -0 -n1 npx markdown-link-check -c .markdownlinkcheck.json",
				`find . -name \*.md -exec markdown-link-check {} \;`,
				"./node_modules/.bin/markdown-link-check README.md",
				"docker run -v .:/tmp:ro --rm -i ghcr.io/tcort/markdown-link-check:stable /tmp/README.md",
				"ghcr.io/tcort/markdown-link-check:3.11.2",
				"pre-commit run --hook-stage manual markdown-link-check --all-files",
				"tcort/github-action-markdown-link-check",
				"tcort/github-action-markdown-link-check@v1",
				"gaurav-nelson/github-action-markdown-link-check",
				"gaurav-nelson/github-action-markdown-link-check@v1",
			},
			shouldNotMatch: []string{
				".github/markdown-link-check-config.json",
				"cat markdown-link-check.fast.json",
				"python utils/run_markdown_link_check.py",
				"<!-- markdown-link-check-disable -->",
				"markdownlint '**/*.md'",
				"avtodev/markdown-lint",
				"lychee .",
				"lycheeverse/lychee-action",
				"UmbrellaDocs/action-linkspector",
				"mymarkdown-link-check README.md",
				"markdown-link-check-extra README.md",
				"tcort/github-action-markdown-link-check-extra@v1",
				"gaurav-nelson/github-action-markdown-link-check-extra@v1",
				"other/github-action-markdown-link-check@v1",
			},
		},
		{
			ruleID: "awesome-bot-rule",
			shouldMatch: []string{
				"awesome_bot README.md", `awesome_bot "${{ matrix.file }}" --allow-redirect --allow-dupe --allow-ssl`,
				`awesome_bot check-unique.txt --allow-ssl --allow 302,429,502 --white-list "$WHITELIST"`,
				"awesome_bot -f README.md,books.md --allow-dupe --allow-redirect --skip-save-results",
				"awesome_bot applications.json -w https://matrix.org --allow 429", "xargs awesome_bot",
				"bundle exec awesome_bot README.md", "./bin/awesome_bot README.md",
				"docker run -ti --rm -v $PWD:/mnt:ro dkhamsing/awesome_bot --allow-dupe --skip-save-results ./templates/ubuntu.md",
				"docker://dkhamsing/awesome_bot:1.20.0", "dkhamsing/awesome_bot:latest",
			},
			shouldNotMatch: []string{
				"./.github/actions/awesomebot-gh-summary-action",
				"$text += \"_Link issues :rocket: powered by [``awesome_bot``](https://github.com/dkhamsing/awesome_bot)_.\"",
				"flutter pub add awesome_bottom_bar", "npx awesome-lint", "lychee README.md", "markdown-link-check README.md",
				"jq . ab-results-README.md-filtered.json",
			},
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
			ruleID: "gradle-wrapper-validation-rule",
			shouldMatch: []string{
				"gradle/actions/wrapper-validation", "gradle/wrapper-validation-action", "Gradle/actions/wrapper-validation",
			},
			shouldNotMatch: []string{
				"gradle/actions/dependency-submission", "gradle/gradle-build-action", "gradle-update/update-gradle-wrapper-action",
				"gradle/actions/wrapper-validation-extra", "Validate Gradle Wrapper", "**/gradle-wrapper.jar",
				`./gradlew wrapper --gradle-version "$GRADLE_VERSION" --distribution-type bin`,
			},
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
		{
			ruleID: "njsscan-rule",
			shouldMatch: []string{
				"njsscan .",
				"njsscan --sarif -o results.sarif src/",
				"ajinabraham/njsscan-action",
				"docker run -v $PWD:/src opensecurity/njsscan /src",
			},
			shouldNotMatch: []string{
				"nodejsscan",
				"semgrep --config p/nodejs .",
				"npm run scan",
			},
		},
		{
			ruleID: "k6-rule",
			shouldMatch: []string{
				"k6 run load.js",
				"k6 cloud script.js",
				"grafana/run-k6-action@v1",
			},
			shouldNotMatch: []string{
				"grafana/setup-k6-action@v1",
				"k6 version",
			},
		},
		{
			// "pester" alone is the module name, which Install-Module and
			// Import-Module load without running anything; only the
			// Invoke-Pester cmdlet runs the tests.
			ruleID: "pester-rule",
			shouldMatch: []string{
				"Invoke-Pester -Path 'pester/*.Tests.ps1' -Output Detailed -CI",
				"Invoke-Pester -Path \"tests\" -CI",
				"Invoke-Pester -Configuration $config",
				"$result = Invoke-Pester -Path test -Output Detailed -PassThru",
				"Invoke-Pester -Output Detailed",
				"pwsh -Command \"Invoke-Pester -Path tests -CI\"",
			},
			shouldNotMatch: []string{
				"Install-Module -Name Pester -RequiredVersion 5.8.0 -Force",
				"Install-PSResource -Name Pester -Version '[5.7.1]'",
				"Import-Module Pester",
				"$config = New-PesterConfiguration",
			},
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
		"opengrep-rule":      {"opengrep test rules/", "opengrep --experimental validate rules/"},
		"sobelow-rule":       {"mix escript.install hex sobelow --force", "mix deps.compile jason sobelow", "mix sobelow --version"},
		"bundler-audit-rule": {"bundle-audit update", "bundle exec bundle-audit update", "bundle audit version", "rake bundle:audit:update"},
		"cargo-vet-rule": {
			"cargo vet init", "cargo vet suggest", "cargo vet fmt", "cargo vet prune",
			"cargo vet aggregate --output-file audits.toml sources.list",
			"cargo vet --output-format=json dump-graph", "cargo vet --locked suggest",
			"cargo vet --manifest-path foo/Cargo.toml suggest",
			"cargo vet explain-audit", "cargo vet gc", "cargo vet renew",
			"cargo vet --help", "cargo vet --version", "cargo +nightly vet init",
		},
		"cargo-deny-licenses-rule":   {"cargo deny check advisories", "cargo deny check bans sources"},
		"cargo-deny-advisories-rule": {"cargo deny check licenses", "cargo deny check bans licenses"},
		"codeql-rule":                {"codeql database create --language=actions"},
		"sonar-rule":                 {"./gradlew build -x test -x sonarqube", "gradle check --exclude-task sonarqube"},
		"zizmor-rule":                {"zizmorcore/zizmor-action"},
		"semgrep-rule": {"semgrep --metrics=off --validate --strict --config=./rules/", "semgrep --test --config rules tests/",
			"semgrep test --strict --config rules/x.yaml", "semgrep login", "semgrep install-semgrep-pro"},
		"npm-audit-rule":       {"npm audit signatures", "npm audit signatures --omit=dev"},
		"ripsecrets-rule":      {"ripsecrets --install-pre-commit", "./bin/ripsecrets --install-pre-commit"},
		"talisman-rule":        {"talisman --checksum 'config/*'", "talisman -c=config/*"},
		"secretlint-rule":      {"npx secretlint --init", "secretlint --init"},
		"tflint-rule":          {"tflint --init", "tflint -v"},
		"kubeconform-rule":     {"kubeconform -v"},
		"lychee-rule":          {"lychee --dump README.md", "lychee --dump-inputs ."},
		"snyk-agent-scan-rule": {"uvx snyk-agent-scan@latest --ci --no-skills ~/.vscode/mcp.json"},
		"trivy-rule":           {"trivy fs --scanners secret .", "trivy repo --scanners misconfig,secret .", "trivy fs --security-checks config .", "trivy image --format cyclonedx alpine", "trivy fs -f spdx ."},
		"typos-rule":           {"npx cspell --report typos ."},
		"unit-test-command-rule": {
			"mvn package -Dmaven.test.skip=true", "./mvnw -B install -DskipTests test", "gradle build -x test", "mvn -B clean install -DskipTests",
			"./gradlew check --exclude-task test", "cargo test --no-run", "cargo test --all --no-run --locked",
			"cargo nextest run --no-run", "cargo nextest run --workspace --no-run --locked",
			"tox -e lint", "tox -e mypy,docs", "tox --envlist=pre-commit",
			"npx playwright test --list", "pnpm exec playwright test --config tests/playwright.config.ts --list --reporter=json",
			"helm unittest --help", "helm unittest --help >/dev/null",
			"gotestsum tool slowest --jsonfile tests.json --threshold 500ms",
			"gotestsum tool ci-matrix --timing-files 'tests/*.log' --partitions 4",
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
