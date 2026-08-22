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
|--------------------------------|--------------------------------|-------------------|
|            CATEGORY            |          DESCRIPTION           |  SUGGESTED TOOLS  |
|--------------------------------|--------------------------------|-------------------|
| Performance and Reliability    | Tools for measuring code       | k6, JMeter,       |
| Testing Tools                  | coverage to ensure testing     | Gatling, Apache   |
|                                | completeness and software      | Bench, Artillery, |
|                                | quality.                       | BlazeMeter        |
|--------------------------------|--------------------------------|-------------------|
```

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
level, with the suggested tools listed in the result's help text. Results carry
no file location, because a missing control is the absence of configuration
rather than a defect on a particular line.

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
brew install zanadir
```

## GitHub Actions
If you're using GitHub Actions, you can use our [Zanadir-based action](https://github.com/MustacheCase/zanadir-action) to run CI\CD scans on your code during your CI workflows.

## Contributors

Zanadir is still in its experimental phase. We are working hard to release the first stable version soon.  
Your feedback and contributions are welcome!
