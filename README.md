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

Zanadir analyzes repositories in the following categories:

- 🛡️ **SCA**: Software Composition Analysis
- 🔐 **Secrets**: Secrets Management
- 📜 **Licenses**: License Compliance
- 🛠️ **EndOfLife**: End-of-Life Software Packages
- 📊 **Coverage**: Test Coverage
- 📊 **Performance Testing**: Test Performance and Reliability
- 🧑‍💻 **Linter**: Code Linting

## Usage Examples

### Basic Usage

Scan a repository for CI/CD improvement suggestions:

```sh
zanadir scan --dir /path/to/your/repo
```

### Output Formats

Zanadir supports two output formats: table (default) and JSON.

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

### Advanced Usage

#### Exclude Specific Categories

Skip certain categories during analysis:

```sh
zanadir scan --dir . --excluded-categories "SCA,Secrets"
```

#### Enforce Mode

Zanadir provides an `--enforce` flag to ensure that all CI/CD suggestions are fulfilled. If any suggestion is not met, the CI pipeline will fail. This helps enforce security best practices and compliance in automated workflows.

```sh
zanadir scan --dir . --enforce
```

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
# Install using Homebrew
brew tap MustacheCase/zanadir
brew install zanadir
```

## GitHub Actions
If you're using GitHub Actions, you can use our [Zanadir-based action](https://github.com/MustacheCase/zanadir-action) to run CI\CD scans on your code during your CI workflows.

## Contributors

Zanadir is still in its experimental phase. We are working hard to release the first stable version soon.  
Your feedback and contributions are welcome!
