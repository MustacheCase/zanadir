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

## Enforce Mode

Zanadir provides an `--enforce` flag to ensure that all CI/CD suggestions are fulfilled. If any suggestion is not met, the CI pipeline will fail. This helps enforce security best practices and compliance in automated workflows.

```sh
zanadir scan --enforce
```

## Installation

You can install Zanadir using Go:

```sh
# Install directly from source
go install github.com/MustacheCase/zanadir@latest
```

## Contributors

Zanadir is still in its experimental phase. We are working hard to release the first stable version soon.  
Your feedback and contributions are welcome!
