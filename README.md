<p align="center">
  <img src="https://github.com/user-attachments/assets/88b976b4-cc46-4706-a3e4-3cfa0e6877d5" alt="zanadir">
</p>

<p align="center">
  <a href="#coverage-score"><img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/MustacheCase/zanadir/badges/zanadir-badge.json" alt="zanadir coverage"></a>
  <a href="https://pkg.go.dev/github.com/MustacheCase/zanadir"><img src="https://pkg.go.dev/badge/github.com/MustacheCase/zanadir.svg" alt="Go Reference"></a>
  <a href="https://codecov.io/gh/MustacheCase/zanadir"><img src="https://codecov.io/gh/MustacheCase/zanadir/branch/main/graph/badge.svg" alt="Test coverage"></a>
</p>

zanadir reads a repository's CI configuration and reports which security and
quality controls are missing, or wired so they cannot fail a build.

## What it answers

- **What is my pipeline missing?** Twelve categories, from dependency
  scanning and secrets detection to SBOMs and workflow hardening, each
  checked against the tools that cover it, with suggestions that fit the
  repository's languages.
- **Does my CI actually block on security findings?** A scanner with
  `continue-on-error`, `--exit-code 0`, `|| true`, a push-only trigger or
  `allow_failure` is present but cannot stop a merge. zanadir grades each
  control *enforcing*, *partial*, *advisory* or *unknown* rather than just
  covered; see [Effectiveness](#effectiveness).
- **Is that check required to merge?** With `--required-checks`, a control
  only counts as enforcing when branch protection or a ruleset requires it.
- **What do I add?** `zanadir fix` prints ready-to-paste CI configuration for
  the gaps, or writes a workflow with `--write`.

It reads GitHub Actions, GitLab CI, CircleCI, Bitbucket Pipelines, Azure
Pipelines and Jenkins, from a checkout (`--dir`) or straight from GitHub
(`--repo`), and reports as a table, JSON, SARIF or Markdown.

```sh
zanadir scan --dir .
zanadir scan --repo github.com/owner/name --output json
zanadir fix --dir .
```

## Installation

Try it on any public repository without installing anything:

```sh
docker run --rm mustachecase/zanadir scan --repo github.com/owner/name
```

To install it, download the archive for Linux, macOS or Windows (amd64 or
arm64) from the [latest release](https://github.com/MustacheCase/zanadir/releases/latest).
`checksums.txt` lists each archive's SHA-256, and every archive has a build
provenance attestation:

```sh
gh attestation verify zanadir_<version>_linux_amd64.tar.gz --repo MustacheCase/zanadir
```

Or with Homebrew (the tap is this repository) or Go:

```sh
brew tap --custom-remote MustacheCase/zanadir https://github.com/MustacheCase/zanadir.git
brew install mustachecase/zanadir/zanadir
go install github.com/MustacheCase/zanadir@latest
```

In GitHub Actions, use [zanadir-action](https://github.com/MustacheCase/zanadir-action).
This workflow reports on every pull request without failing it:

```yaml
# .github/workflows/zanadir.yml
name: zanadir
on: [pull_request]
permissions:
  contents: read
jobs:
  zanadir:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: mustachecase/zanadir-action@v1.22
        with:
          dir: .
```

When you are ready to gate, add `enforce: true` (fail on any uncovered
category), `fail-on` (only the categories you name) or `fail-on-weakened`. To
post the report as a pull request comment, see
[Posting the report to a pull request](#posting-the-report-to-a-pull-request).

## zanadir and OpenSSF Scorecard

[OpenSSF Scorecard](https://github.com/ossf/scorecard) and zanadir answer
different questions, and work well together.

| | OpenSSF Scorecard | zanadir |
| --- | --- | --- |
| Question | How healthy are this project's security practices? | Which CI controls are missing, and can the ones present fail a build? |
| Scope | 20 checks across the project: branch protection, code review, maintenance, pinned dependencies, token permissions, signed releases, known vulnerabilities, fuzzing and more | 12 categories of CI controls, each matched against the many tools that provide it |
| Reads | The forge's API: GitHub and GitLab repositories, recent pull requests and check runs | The CI configuration: six CI platforms, the Makefiles, Taskfiles, tox, composite actions and templates its jobs call |
| SAST, for example | Looks for CodeQL or SonarCloud; its docs note other tools may score low | Recognises Semgrep, CodeQL, gosec, Bandit, Brakeman, SonarQube and others, and grades whether each can fail the build |
| Result | A 0-10 score per check and overall | Covered or not per category, graded enforcing, partial, advisory or unknown, with fixes |

Use Scorecard for a project-wide security posture, and zanadir to find out
whether the CI you have would actually stop a bad change, and what to add.

## Using zanadir with AI coding agents

To have a coding agent check CI with zanadir before changing it, add this to
the repository's `AGENTS.md`, `CLAUDE.md` or equivalent:

```markdown
## CI changes

Before adding or changing CI configuration, run `zanadir scan --dir . --output json`
(install: https://github.com/MustacheCase/zanadir#installation). It lists the
missing CI controls and grades each present one: `enforcing` can fail a merge,
`partial`, `advisory` and `unknown` cannot or may not. Fix advisory controls before
adding tools, and use `zanadir fix --dir .` for configuration that fits this
repository. Re-run the scan to confirm the change.
```

In Claude Code, the zanadir plugin does this as a skill: it scans, reads your
CI configuration, recommends fixes in priority order, and edits files only
after you choose.

```
/plugin marketplace add MustacheCase/zanadir
/plugin install zanadir@mustachecase
```

Then ask Claude Code to review or harden your CI, or run `/zanadir:improve-ci`.

[`llms.txt`](llms.txt) summarises zanadir for agents that read the repository, and is
served at [mustachecase.github.io/llms.txt](https://mustachecase.github.io/llms.txt)
for those that look at a site's root.

## Features

- 📂 **Scan**: Analyze the repository for CI/CD enhancement suggestions, including security services and best practices.
- 🔍 **CI Analysis**: Grades whether each control can actually fail a build, not only whether it is present.
- 🛠️ **Fix**: Prints, or writes as a workflow, CI configuration for the categories a scan reports as missing.
- 🚀 **Open Source**: Contributions are welcome to enhance Zanadir's capabilities!

## Supported CI Actions

Zanadir currently supports:

- GitHub Actions
- CircleCI
- GitLab
- Bitbucket Pipelines
- Azure Pipelines
- Jenkins

Future work will include support for:

- Tekton

## Categories We Suggest

Zanadir analyzes repositories in the following categories. The name in bold is
the exact value to pass to `--excluded-categories` (matching is
case-insensitive):

- 🛡️ **SCA**: Software Composition Analysis
- 🔐 **Secrets Detection**: Secrets Management
- 📜 **License Compliance**: License Compliance
- 🛠️ **End Of Life**: End-of-Life Software Packages
- 📊 **Coverage**: Test Coverage
- 📊 **Performance Testing**: Test Performance and Reliability
- 🧑‍💻 **Linter**: Code Linting
- 🧪 **Unit Tests**: Automated Unit Testing
- 🔎 **SAST**: Static Application Security Testing
- 🏗️ **IaC Security**: Infrastructure-as-Code Misconfiguration Scanning
- 🔗 **Supply Chain**: SBOM Generation, Signing and Provenance
- 🧱 **Workflow Security**: Hardening of the pipeline definition itself

## Usage Examples

### Basic Usage

Scan a repository for CI/CD improvement suggestions:

```sh
zanadir scan --dir /path/to/your/repo
```

Or scan a GitHub repository without cloning it:

```sh
zanadir scan --repo github.com/owner/name
zanadir scan --repo owner/name@v2.0.0     # a branch, tag or commit
zanadir fix --repo owner/name
```

`--repo` downloads the repository's archive into a temporary directory, scans
it exactly as a checkout, and removes it. Only the files zanadir reads keep
their contents. GitHub's archive leaves out whatever `.gitattributes` marks
`export-ignore`, which often includes `.github`, so zanadir fetches those
files separately rather than report a repository with no CI. Public
repositories need no token; set `GITHUB_TOKEN` (or `GH_TOKEN`) to reach
private ones or raise GitHub's rate limit. GitLab and Bitbucket repositories
still need a clone.

### Output Formats

Zanadir supports three output formats: table (default), JSON and SARIF.

#### Table Output (Default)

```sh
zanadir scan --dir . --output table
```

**Sample Output:**
```
Coverage 10/11 - 1 category needs attention:

|--------------------------------|--------------------------------|-------------------|
|            CATEGORY            |          DESCRIPTION           |  SUGGESTED TOOLS  |
|--------------------------------|--------------------------------|-------------------|
| Performance and Reliability    | Tools for load, stress and     | k6, JMeter,       |
| Testing Tools                  | reliability testing, to verify | Gatling, Apache   |
|                                | a system holds up under        | Bench, Artillery, |
|                                | expected and peak traffic.     | BlazeMeter        |
|--------------------------------|--------------------------------|-------------------|
```

When nothing is missing, the scan says so rather than printing an empty table:

```
Coverage 11/11 - all categories are covered.
```

The headline is bold on an interactive terminal. Colour is omitted when the
output is piped or written with `--output-file`, and when `NO_COLOR` is set.

#### JSON Output

```sh
zanadir scan --dir . --output json
```

One object with the score, the CI systems whose configuration was found, the
uncovered categories and the tools that cover them, the covered categories whose tooling cannot fully protect the
repository, and the categories left out of the score. Every list is an array,
empty rather than `null` when there is nothing in it.

```json
{
  "score": { "covered": 4, "total": 11 },
  "platforms": ["github"],
  "uncovered": [
    {
      "id": "Secrets Detection",
      "name": "Data Leakage & Secrets Detection",
      "description": "Tools that help detect sensitive information ...",
      "suggestions": [
        {
          "name": "Gitleaks",
          "repository": "https://github.com/gitleaks/gitleaks",
          "description": "..."
        }
      ]
    }
  ],
  "weakened": [
    { "category": "SCA", "verdict": "advisory", "location": ".github/dependabot.yml" }
  ],
  "notApplicable": ["IaC Security"]
}
```

`language` appears on a suggested tool only when it is specific to one
language, and `location` on a weakened category only when it points inside
the repository. Before 0.10.0, `--output json` printed only the bare array that
is now `uncovered`, with capitalised keys.

#### SARIF Output

```sh
zanadir scan --dir . --output sarif
```

SARIF is the standard interchange format for static analysis results. Emitting
it lets uncovered categories show up in GitHub's Security tab, and in any other
SARIF-consuming platform, alongside your other scanners.

Each uncovered category becomes one SARIF rule and one result at `warning`
level, with the suggested tools listed in the result's help text. Results are
anchored at a CI configuration file — the place the missing tool would be
added. A missing control is not a defect on a particular line, but every SARIF
consumer expects a location, and GitHub code scanning rejects a report whose
results have none.

#### Markdown Output

```sh
zanadir scan --dir . --output markdown
```

Markdown is the format for putting a scan in front of people rather than in a
file. Tools link to their repositories, and the report opens with a hidden
marker so a workflow can update the comment it posted last time instead of
adding a new one on every push.

**Sample Output:**
```markdown
<!-- zanadir-report -->
### zanadir

Coverage 10/11 - 1 category needs attention:

| Category | Description | Suggested tools |
| --- | --- | --- |
| Performance and Reliability Testing Tools | Tools for load, stress and reliability testing. | [k6](https://github.com/grafana/k6), [JMeter](https://github.com/apache/jmeter) |
```

When nothing is missing the table is omitted, leaving a single line.

#### Writing the report to a file

Use `--output-file` to write the report to a path instead of stdout. This works
with every format, and is the reliable way to produce a machine-readable report:
redirecting stdout captures the debug log as well, so `--output sarif --debug >
report.sarif` yields a file that is not valid JSON.

```sh
zanadir scan --dir . --output sarif --output-file zanadir.sarif
```

To publish the report from a GitHub Actions workflow:

```yaml
- name: Run zanadir
  run: zanadir scan --dir . --output sarif --output-file zanadir.sarif

- name: Upload SARIF
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: zanadir.sarif
```

Uncovered categories then appear in the repository's **Security** tab alongside
your other scanners.

### Posting the report to a pull request

SARIF puts uncovered categories in the **Security** tab, which people visit
occasionally. A pull request comment is where they already are.

```yaml
permissions:
  contents: read
  pull-requests: write

steps:
  - name: Run zanadir
    run: zanadir scan --dir . --output markdown --output-file zanadir.md

  - name: Post the report
    run: gh pr comment "$NUMBER" --body-file zanadir.md --edit-last --create-if-none
    env:
      GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      NUMBER: ${{ github.event.pull_request.number }}
```

`--edit-last` keeps one comment up to date instead of adding one per push. It
matches on the last comment by the same user, so if something else comments as
`github-actions` too, match the marker instead:

```yaml
  - name: Find the previous report
    uses: peter-evans/find-comment@v4
    id: previous
    with:
      issue-number: ${{ github.event.pull_request.number }}
      body-includes: "<!-- zanadir-report -->"

  - name: Post the report
    uses: peter-evans/create-or-update-comment@v5
    with:
      comment-id: ${{ steps.previous.outputs.comment-id }}
      issue-number: ${{ github.event.pull_request.number }}
      body-path: zanadir.md
      edit-mode: replace
```

A pull request from a fork gets a read-only token, so the comment step cannot
post. Guard it with
`if: github.event.pull_request.head.repo.full_name == github.repository`
rather than reaching for `pull_request_target`, which would run your workflow
with write access to a fork's code.

## Coverage Score

Every scan is summarised as a coverage score: the categories that apply to the
repository, minus the ones still uncovered.

Three rules keep the number honest:

- **Excluded categories leave the denominator.** A repository is not marked
  down for a category it has declared irrelevant with `--excluded-categories`.
- **So do categories with nothing to scan.** A repository with no Terraform,
  Helm chart, Kubernetes manifest or Dockerfile is not told to add Checkov,
  and is scored out of ten rather than eleven. The scan says which:

  ```
  IaC Security is not applicable and is out of the score: nothing in this
  repository needs it.
  ```

  Two categories are ruled out this way. **IaC Security** needs
  infrastructure to scan, and the detection is deliberately broad — a
  directory called `infra`, `k8s` or `charts` is enough to keep it.
  **Workflow Security** needs GitHub Actions, because every tool in it reads
  that format, so a GitLab-only repository is not told to add actionlint.

  The rest apply to any codebase, and the ones that arguably do not — load
  testing a library, an SBOM for something that ships nothing — cannot be told
  apart from the ones that do, so guessing would rule out categories a
  repository does need.
- **Baseline-accepted gaps still count as uncovered.** A baseline records that
  a gap is tolerated, not that it was closed. A score that rose because someone
  committed a file would measure nothing.

### README Badge

`--badge` writes the score as a [shields.io endpoint][shields] file:

```sh
zanadir scan --dir . --badge zanadir-badge.json
```

```json
{
  "schemaVersion": 1,
  "label": "zanadir",
  "message": "10/11",
  "color": "green"
}
```

Publish that file from CI and point a badge at it. Put it on a branch of its
own rather than committing it to the default branch: a protected default
branch only takes changes through a pull request, so a workflow that pushes
the badge there is rejected.

```markdown
![zanadir](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/OWNER/REPO/badges/zanadir-badge.json)
```

A job that keeps it current on every push to the default branch, publishing
to a `badges` branch it creates on the first run:

```yaml
badge:
  if: github.event_name == 'push'
  runs-on: ubuntu-latest
  permissions:
    contents: write
  steps:
    - uses: actions/checkout@v7

    - name: Run zanadir
      run: zanadir scan --dir . --badge zanadir-badge.json

    - name: Publish the badge when it moved
      run: |
        set -euo pipefail
        cp zanadir-badge.json "$RUNNER_TEMP/zanadir-badge.json"
        rm -f zanadir-badge.json
        git config user.name github-actions
        git config user.email github-actions@github.com
        if git fetch --depth=1 origin badges 2>/dev/null; then
          git switch --discard-changes -C badges FETCH_HEAD
        else
          git switch --orphan badges
        fi
        cp "$RUNNER_TEMP/zanadir-badge.json" zanadir-badge.json
        git add zanadir-badge.json
        if git diff --cached --quiet; then exit 0; fi
        git commit -m "chore: update zanadir badge"
        git push origin badges
```

The `badges` branch holds that one file and nothing else. If a ruleset
covers every branch rather than only the default one, exclude `badges` from
it.

The colour tracks the score: green at 80% or above, yellow at 60%, orange at
40%, red below that, and bright green only at full coverage.

[shields]: https://shields.io/badges/endpoint-badge

## Effectiveness

Presence is not protection. A scanner wired up so it can never fail a build
leaves a repository worse off than one with no scanner at all, because
everybody believes the control is there. All of these used to report
**covered**:

```yaml
- uses: gitleaks/gitleaks-action@v2
  continue-on-error: true          # the job stays green whatever it finds
```

```yaml
on:
  schedule: [{cron: "0 3 * * *"}]  # nothing is blocked at merge time
```

```yaml
on:
  push: {branches: [main]}         # pull requests are never checked
```

A scan now grades each control it finds and reports the ones that protect less
than their presence suggests:

```
3 covered categories are weakened:
  Linter is partial (.github/workflows/pushonly.yml)
  SCA is advisory (.github/workflows/nightly.yml)
  Secrets Detection is advisory (.github/workflows/ci.yml)
```

| Verdict | Meaning |
|---|---|
| `enforcing` | the control can fail the build on the path that gates a change |
| `partial` | it can fail a build, but not on that path - pushes only, say |
| `advisory` | it runs, and cannot block anything |
| `unknown` | the configuration cannot be read well enough to say |

A `pull_request` trigger gates only if it runs on the change itself and on
pull requests to the default branch. `types: [closed]` runs after the merge,
so it grades like a push. A `branches:` or `branches-ignore:` filter that
leaves out the default branch makes the control `unknown`. The default branch
is set outside the repository: zanadir reads it from the checkout's
`origin/HEAD`, which `git clone` sets, from GitHub with `--repo` or
`--required-checks`. Where none of those has it, as in most CI checkouts,
`main`, `master`, `trunk` and `develop` stand in for it, or whichever of them
the filter names.

Supply Chain is graded against a different path. An SBOM, a signature or a
provenance attestation describes what ships rather than looking for something
to block, so it belongs where the artifact is published: running on a release,
a push, a tag, a pull request or a workflow chained after one is `enforcing`.
A manual run may be the release itself, which the file cannot tell, so it is
`unknown`. Only a schedule, detached from anything that ships, is `advisory`,
and `continue-on-error` is `advisory` here as everywhere: a signing step that
can fail silently publishes unsigned artifacts.

A weakened control does not fail a scan unless you ask it to. `--enforce` and
`--fail-on` act only on categories with no tooling; `--fail-on-weakened` is
what turns a weak control into a failure:

```sh
zanadir scan --dir . --fail-on-weakened
```

```
Enforcement failed: controls that cannot fail a build: Secrets Detection (advisory)
```

The two are independent. `--enforce` on its own still ignores weakened
controls, so upgrading does not start failing builds that passed before.
`--fail-on` narrows both.

A baseline accepts a weakness **at the verdict it had when accepted**:

```yaml
version: 1
categories:
  - Performance Testing
weakened:
    Secrets Detection: partial
```

So accepting `partial` does not also accept it becoming `advisory` later. If
the control gets worse than its recorded verdict, the scan fails again.
`--write-baseline` records the verdicts for you.

Some of this is generic. `continue-on-error` defeats any control, and so do
its shell spellings: `gitleaks detect || true`, `|| echo failed`, and running
the tool as the condition of an `if` or a `!`. A `|| exit 1` that fails again
is fine. So does a workflow that only runs on a schedule. The rest is specific
to one tool, and a rule can say so:

```yaml
  - id: "tfsec-rule"
    regex: "(?i)\\btfsec\\b"
    weakenedWhen:
      - runMatches: "--soft-fail\\b"
        verdict: "advisory"
      - input: "soft_fail"
        equals: "true"
        verdict: "advisory"
```

A check is either a regex over the step's shell command or one action input
compared to a value - deliberately not an expression language. Covered today:
`trivy --exit-code 0`, `gitleaks --exit-code 0`, `tfsec` and `checkov`
`--soft-fail` / `soft_fail: true`, and `anchore/scan-action` with
`fail-build: false`.

Two deliberate silences. An `if:` condition is never interpreted, because it
can reference arbitrary context and reading one wrong is worse than admitting
it cannot be read - the control is reported `unknown` instead. And a category
is judged by its strongest control, so one properly wired scanner keeps the
category quiet however many weak ones sit beside it.

### Required checks

A scanner can run on every pull request and still block nothing, because its
check is not required to merge. That lives in the repository's rulesets and
branch protection, not its files, so it is opt-in:

```sh
zanadir scan --dir . --required-checks
zanadir scan --repo owner/name --required-checks
```

zanadir reads the default branch's required checks from GitHub (public
repositories need no token; `GITHUB_TOKEN` or `GH_TOKEN` is used if set). A
GitHub Actions control that would be `enforcing` stays so only when its job's
check is required, matrix (`build (ubuntu-latest)`) and reusable-workflow
(`codeql / Analyze`) names included; otherwise it is `partial`:

```
2 covered categories are weakened:
  SAST is partial (.github/workflows/codeql.yml): its check is not required to merge
  Secrets Detection is partial (.github/workflows/secrets.yml): its check is not required to merge

Merge settings for main:
  Required to merge: build (ubuntu-latest), build (macos-latest).
  Problem: ruleset "checks must pass" requires "Run tests" but applies to no branch.
```

It also names settings that look like protection and are not: a ruleset that
requires checks but targets no branch or only evaluates, and a required
GitHub Actions check that no workflow job reports. Checks other apps report,
such as DCO, are never judged. If the settings cannot be read, the scan warns
and grades without them. Only GitHub is read; on other platforms the merge
settings are still outside what zanadir can see.

### Controls declared outside the pipeline

Dependabot and Renovate are detected from their configuration -
`.github/dependabot.yml`, `renovate.json` and the other names Renovate
accepts - not from a CI job, because that is not where they live.

They are graded `advisory`. Both run on the forge's own schedule and open pull
requests; neither can fail a build. So a repository whose only dependency
control is Dependabot is reported as covered **and** weakened, which is a more
useful answer than either "you have no SCA" or "you are covered":

```
1 covered category is weakened:
  SCA is advisory (.github/dependabot.yml)
```

### Platform coverage

GitHub Actions is graded in full. GitLab CI is graded for the two signals that
are unambiguous there - `allow_failure: true`, which is `continue-on-error`
under another name, and `when: manual`, a job that only runs when somebody
asks.

GitLab's `include:` is followed. A local file (`local:` or a bare path,
wildcards allowed) is read as more of the same configuration. GitLab's own
templates and CI/CD components cannot be read from the repository, so they are
recognised by name: `Jobs/SAST.gitlab-ci.yml` or
`components/sast/sast@...` counts as SAST, and likewise for Secret Detection,
Dependency and Container Scanning, SAST-IaC, License Scanning, Code Quality and
the performance-testing templates. A `project:` or `remote:` include is
skipped. A job zanadir cannot read is skipped rather than failing the scan.

GitLab's `rules:`, `only:` and `except:` are deliberately not interpreted.
Whether a branch pipeline gates a merge depends on a project setting that is
not in the repository, so reading them would be guessing. A GitLab job with
neither weakening field is reported `unknown`, which says nothing rather than
something wrong. CircleCI has no equivalent signal and is not graded.

Bitbucket Pipelines splits its file by trigger, so most of it is graded:

| Where the tool runs | Verdict |
| --- | --- |
| `pull-requests:` | `enforcing` |
| `branches:` for one named branch, such as `main` | `partial` - it runs after the merge |
| `tags:`, `custom:`, or a step or stage with `trigger: manual` | `advisory` |
| `after-script:` | `advisory` - a failing after-script does not fail the step |
| `default:`, or a wildcard branch pattern | `unknown` |

`default:` also runs on a pull request's source branch, and whether that build
blocks the merge is a merge check configured outside the repository, so it is
left `unknown` for the same reason as GitLab's branch pipelines. Pipes are read
like GitHub actions: `pipe: snyk/snyk-scan` counts as Snyk, and
`DONT_BREAK_BUILD: "true"` grades it `advisory`.

A Jenkinsfile (and variants beside it such as `Jenkinsfile.release`) is read
as Groovy tokens, not run. `sh`, `bat` and `powershell` steps are read like
`run:`, and other steps such as `snykSecurity`, `dependencyCheck`,
`withSonarQubeEnv` or `junit` like actions, with their named arguments as
inputs. Which branches and pull requests build is configured on the Jenkins
job, not in the file, so a control is `unknown` unless the file itself
weakens it:

| In the Jenkinsfile | Verdict |
| --- | --- |
| inside `catchError(buildResult: 'SUCCESS')` or `buildResult: null` | `advisory` - the build stays green |
| a step option such as `snykSecurity(failOnIssues: false)` or `waitForQualityGate abortPipeline: false` | `advisory` |
| inside `when { }`, `if`, `warnError` or `catchError(buildResult: 'UNSTABLE')`, or `sh(returnStatus: true)` | `unknown` |

A shared-library step configured only by assignments, such as
`mavenBuild { dependencyCheck = 'enabled' }`, cannot be read.

Azure Pipelines is read from `azure-pipelines.yml` (or `.yaml`) at the
repository root, following `template:` and `extends:` references to files in
the same repository. A template in another repository (`path@alias`) or one
named by an expression cannot be read and is skipped.

| Where the tool runs | Verdict |
| --- | --- |
| a pipeline whose `pr:` names branches | `enforcing` |
| `continueOnError: true` on the step or its job | `advisory` |
| `trigger: none`, `pr: none` and only `schedules:` | `advisory` |
| any `condition:` other than `succeeded()` | `unknown` |
| a pipeline with no `pr:` | `unknown` |

With no `pr:`, an Azure Repos pull request is validated by a branch policy
outside the repository, so the file cannot say whether the pipeline gates it.
`succeeded()` is ignored because it is the default every step already has. A
step with `enabled: false` never runs and counts for nothing. Tasks are read
like actions, with `inputs:` as their inputs, so `SnykSecurityScan` with
`failOnIssues: false` grades `advisory`.

In SARIF a weakened control is a `note`, pointing at the file that produced
the verdict, while a missing category stays a `warning`.

## Language-Aware Suggestions

Zanadir detects which languages a repository uses and suggests only tools that
apply to it — a Go project is not offered ESLint, and a JavaScript project is
not offered Pylint. Tools that work regardless of ecosystem (Trivy, Gitleaks,
Codecov, and so on) are always suggested.

Detection is based on dependency manifests in the repository root and its
immediate subdirectories, so common monorepo layouts work:

| Language   | Detected from                                              |
|------------|------------------------------------------------------------|
| Go         | `go.mod`                                                   |
| Python     | `requirements.txt`, `pyproject.toml`, `setup.py`, `Pipfile` |
| JavaScript | `package.json`                                             |
| Ruby       | `Gemfile`, `*.gemspec`                                     |
| Java       | `pom.xml`, `build.gradle`, `build.gradle.kts`              |
| Rust       | `Cargo.toml`                                               |
| PHP        | `composer.json`                                            |
| C#         | `*.csproj`, `*.sln`                                        |

`node_modules`, `vendor` and other dependency directories are skipped, since
their manifests describe code the project did not write. If no language is
recognised, every tool is suggested, exactly as before.

### Advanced Usage

#### Exclude Specific Categories

Skip certain categories during analysis:

```sh
zanadir scan --dir . --excluded-categories "SCA,Secrets Detection"
```

Category names must match the list above. An unrecognized name is rejected with
an error rather than being silently ignored.

#### Enforce Mode

Zanadir provides an `--enforce` flag to ensure that all CI/CD suggestions are fulfilled. If any suggestion is not met, the CI pipeline will fail. This helps enforce security best practices and compliance in automated workflows.

```sh
zanadir scan --dir . --enforce
```

The category that failed the build is printed to stderr:

```
Enforcement failed: uncovered categories: SCA, Secrets Detection
```

Exit codes: `0` when the scan passes, `1` when enforcement fails or the scan
cannot run (an unreadable directory or configuration), and `2` for a command
line zanadir does not understand, such as a mistyped flag. A typo therefore
fails the build instead of passing it with nothing enforced.

#### Failing on Specific Categories

`--enforce` is all-or-nothing, which makes it hard to adopt on an existing
repository: a single uncovered category fails every build. Use `--fail-on` to
enforce only the categories you care about, while the rest stay informational:

```sh
zanadir scan --dir . --fail-on "SCA,Secrets Detection"
```

`--fail-on` implies enforcement, so `--enforce` is not needed alongside it.
Category names are matched case-insensitively.

#### Baseline

A baseline records the gaps a repository knowingly accepts. Those categories
are still reported, but they no longer fail a scan — so enforcement can be
switched on before everything is fixed, and only *new* gaps break the build.

Generate one from the current state:

```sh
zanadir scan --dir . --write-baseline
```

This writes `.zanadir-baseline.yaml` (override with `--baseline <path>`) and
exits successfully:

```yaml
# zanadir baseline: categories that are uncovered but accepted.
# These are still reported; they just do not fail a scan.
# Regenerate with: zanadir scan --dir . --write-baseline
version: 1
categories:
    - Coverage
    - Performance Testing
```

Then enforce against it. The scan passes while the repository's gaps match the
baseline, and fails as soon as a new one appears:

```sh
zanadir scan --dir . --enforce --baseline .zanadir-baseline.yaml
```

Commit the baseline so the accepted gaps are reviewable, and shrink it over
time.

#### Debug Mode

Get detailed logging information:

```sh
zanadir scan --dir . --debug
```

#### Config File

`--config` (`-c`) reads flag values from a YAML file, so a team stops
repeating them on every invocation. Each key is the name of the flag it sets:

```yaml
# .zanadir.yml
excluded-categories: [Performance Testing]
enforce: true
fail-on-weakened: true
required-checks: true
baseline: .zanadir-baseline.yaml
badge: zanadir-badge.json
output: markdown
```

```sh
zanadir scan --dir . -c .zanadir.yml
zanadir scan --dir . -c .zanadir.yml --output json   # the command line wins
```

- A flag given on the command line overrides the same key in the file.
- An unknown key is an error, so a misspelt `exclude-categories` fails the
  scan instead of silently scanning without the exclusion.
- Relative paths (`baseline`, `badge`, `output-file`) are resolved against the
  directory holding the config file, not the directory you run from.
- `zanadir fix -c` reads the same file and uses only `excluded-categories` and
  `debug`.
- `dir` and `write-baseline` stay on the command line: one says what to scan,
  the other is a one-off action rather than a setting.

#### Custom Rules

zanadir only recognises the tools in its catalogue, so an in-house scanner
reads as a missing category. Teach it with `rules:` in the config file, in the
same shape as the [built-in rules](rules/storage):

```yaml
rules:
  - id: acme-scan
    applyOn: [Job.Run]          # Artifact.Name, Job.Name, Job.Package, Job.Run
    categories: [SAST]
    regex: '\bacme-scan\b'
    unless: '--list-rules'      # optional: a match that does not count, e.g. a command that does not scan
    weakenedWhen:               # optional: configurations that cannot fail a build
      - runMatches: '--report-only'
        verdict: advisory       # advisory or partial
```

Custom rules add to the catalogue and never replace a built-in rule, so an
`id` that is already taken is an error, as is a regex that does not compile or
an unknown category. A rule that matches counts as coverage and is graded like
any other, so `--fail-on-weakened` catches `acme-scan --report-only` too.
`zanadir fix` uses the same rules to decide what is uncovered.

#### Complete Example

```sh
# Scan with all options
zanadir scan \
  --dir /path/to/repo \
  --output json \
  --excluded-categories "Linter" \
  --enforce \
  --debug
```

## `zanadir fix`

A scan tells you what is missing. `fix` prints the configuration to add:

```sh
zanadir fix --dir .
```

```
Infrastructure as Code Security is not covered. Add to .github/workflows/security.yml:

  - name: Scan infrastructure as code
    uses: bridgecrewio/checkov-action@v12
    with:
      directory: .

  https://github.com/bridgecrewio/checkov
```

It prints only; no file is touched. The CI platform is detected from the
repository, and snippets are emitted for GitHub Actions, GitLab CI, Bitbucket Pipelines, Azure Pipelines or Jenkins
accordingly.

Not every suggested tool has a template. A category whose tools are all
untemplated is skipped rather than emitting something half-right, so `fix`
covers fewer categories than `scan` reports. Templates live in
`fixer/templates.yaml` and pin actions to a major tag, or to a release
where the action publishes no major tag, never to a branch; adding one is a
self-contained contribution.

### Generating a workflow

`--write` generates a standalone `.github/workflows/zanadir-suggested.yml`
instead of printing:

```sh
zanadir fix --dir . --write
```

```
Wrote .github/workflows/zanadir-suggested.yml (9 categories).
It runs as its own job, so it repeats checkout and adds a check to pull requests.
```

Existing workflows are never edited, so nothing you wrote can be damaged; the
worst case is a file you delete. Re-running overwrites the generated file and
produces the same result, and zanadir refuses to overwrite a file at that path
that it did not generate.

That standalone job is the trade-off: it repeats checkout and adds its own
entry to the checks list, which is noisier than adding the steps to a job you
already have. Move them into your own workflow whenever you prefer.

`--write` currently generates GitHub Actions only. On other platforms, run
without it and paste the snippets.

`--excluded-categories` works as it does for `scan`.

## Contributors

Feedback, issues and pull requests are welcome; see [CONTRIBUTING.md](CONTRIBUTING.md).
