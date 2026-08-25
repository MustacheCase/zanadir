<p align="center">
  <img src="https://github.com/user-attachments/assets/88b976b4-cc46-4706-a3e4-3cfa0e6877d5" alt="zanadir">
</p>

## Features

- 📂 **Scan**: Analyze the repository for CI/CD enhancement suggestions, including security services and best practices.
- ❓ **Help**: Get details on available commands and usage.
- 🔍 **CI Analysis**: Examines the repository's Continuous Integration (CI) setup and suggests improvements for security and best practices.
- 🚀 **Open Source**: Contributions are welcome to enhance Zanadir's capabilities!

## Supported CI Actions

Zanadir currently supports:

- GitHub Actions
- CircleCI
- GitLab

Future work will include support for:

- Bitbucket

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

## Usage Examples

### Basic Usage

Scan a repository for CI/CD improvement suggestions:

```sh
zanadir scan --dir /path/to/your/repo
```

### Output Formats

Zanadir supports three output formats: table (default), JSON and SARIF.

#### Table Output (Default)

```sh
zanadir scan --dir . --output table
```

**Sample Output:**
```
1 category needs attention:

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
All categories are covered - no suggestions.
```

The headline is bold on an interactive terminal. Colour is omitted when the
output is piped or written with `--output-file`, and when `NO_COLOR` is set.

#### JSON Output

```sh
zanadir scan --dir . --output json
```

**Sample Output:**
```json
[
  {
    "ID": "Performance Testing",
    "Name": "Performance and Reliability Testing Tools",
    "Description": "Tools for measuring code coverage to ensure testing completeness and software quality.",
    "Suggestions": [
      {
        "Name": "k6",
        "Repository": "https://github.com/grafana/k6",
        "Description": "Grafana k6 is an open-source, developer-friendly, and extensible load testing tool. k6 allows you to prevent performance issues and proactively improve reliability.",
        "Language": ""
      },
      {
        "Name": "JMeter",
        "Repository": "https://github.com/apache/jmeter",
        "Description": "An Apache project designed to load test functional behavior and measure performance, with support for various protocols and servers.",
        "Language": ""
      }
    ]
  }
]
```

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
Data Leakage & Secrets Detection is not covered. Add to .github/workflows/security.yml:

  - name: Detect secrets
    uses: gitleaks/gitleaks-action@v2
    env:
      GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

  https://github.com/gitleaks/gitleaks
```

It prints only; no file is touched. The CI platform is detected from the
repository, and snippets are emitted for GitHub Actions or GitLab CI
accordingly.

Not every suggested tool has a template. A category whose tools are all
untemplated is skipped rather than emitting something half-right, so `fix`
covers fewer categories than `scan` reports. Templates live in
`fixer/templates.yaml` and pin actions to major tags; adding one is a
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

## Installation

You can install Zanadir using Go:

```sh
# Install directly from source
go install github.com/MustacheCase/zanadir@latest
```

Or using Homebrew:

```sh
# Install using Homebrew (custom tap)
brew tap --custom-remote MustacheCase/zanadir https://github.com/MustacheCase/zanadir.git
brew install mustachecase/zanadir/zanadir
```

The tap is this repository, so the formula lives in `Formula/zanadir.rb`.

## GitHub Actions
If you're using GitHub Actions, you can use our [Zanadir-based action](https://github.com/MustacheCase/zanadir-action) to run CI\CD scans on your code during your CI workflows.

## Contributors

Zanadir is still in its experimental phase. We are working hard to release the first stable version soon.  
Your feedback and contributions are welcome!
