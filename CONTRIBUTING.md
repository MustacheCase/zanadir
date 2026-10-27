# Contributing to Zanadir

Thank you for your interest in contributing to Zanadir! This document provides guidelines and instructions for contributing to this project.

## Prerequisites

- Go 1.26 or later
- Git
- Basic understanding of Go programming language

## Getting Started

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/zanadir.git
   cd zanadir
   ```
3. Add the original repository as upstream:
   ```bash
   git remote add upstream https://github.com/MustacheCase/zanadir.git
   ```

## Development Workflow

1. Create a new branch for your feature or bugfix:
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/your-bugfix-name
   ```

2. Make your changes and commit them with clear, descriptive commit messages:
   ```bash
   git commit -m "Description of your changes"
   ```

3. Push your changes to your fork:
   ```bash
   git push origin feature/your-feature-name
   ```

4. Create a Pull Request from your fork to the main repository

## Code Style and Standards

- Follow the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use `gofmt` to format your code
- Write tests for new features and bug fixes
- Ensure all tests pass before submitting a PR

## Testing

Run the test suite:
```bash
go test ./...
```

## Working with an AI agent

The repository's `.mcp.json` gives Claude Code and other MCP clients zanadir's
own `scan` and `fix` tools, built from the source you are working on
(`go run . mcp`). Approve the zanadir server when the session starts, then ask
the agent to scan a repository, or this one, to see a change's effect.

## Adding a Tool to the Catalogue

zanadir's suggestions (`suggester/suggestions.yaml`) are read by people and,
through `zanadir mcp` and the plugin, by AI agents that act on them. A tool in
the catalogue is recommended to every repository missing its category, so
every tool gets in the same way.

### What a new tool needs

- **It runs in CI unattended.** It installs and finishes without a person.
- **It can fail the build**, by default or with a flag. zanadir asks whether a
  control can stop a merge, and a tool that can only print findings cannot.
- **It is free to use on a public repository.** Open source or a free tier
  both count. A tool that needs a paid plan to try is not suggested.
- **It is maintained.** Its source repository is public, not archived, and has
  had a release in the last year. A weekly check (`tools/cataloguehealth`)
  flags entries that stop meeting this.
- **zanadir recognises it.** A rule in `rules/storage/` matches the forms the
  tool is usually run in: its action, its image, its command. Without one,
  zanadir keeps suggesting the tool to repositories that already run it. Add
  regex tests for what should and should not match.
- **Its fix snippet has been run.** A template in `fixer/templates.yaml` is
  pinned to a release, and the PR says what happened when you ran it on a real
  repository, including that a finding fails the build.

### What does not count

- **Payment, sponsorship or partnership.** There is no paid placement.
- **Stars or popularity alone.**

### Order

`zanadir fix` pastes the first tool in a category that has a snippet for the
repository's CI system, so order matters. It is decided by fit for the
repositories that will see the suggestion, for example whether the tool needs
an account, and every request to move a tool up is judged the same way.

### If you work on the tool

Maintainers adding their own tool are welcome; glsec arrived this way. Say so
in the PR description. Open an issue first describing the tool and the
categories it covers, so the fit is agreed before the code.

### Removal

A tool is removed when it stops meeting these criteria: its repository is gone
or archived, it can no longer fail a build, or it has stopped being
maintained. Entries flagged by the weekly check are reviewed by hand, not
removed automatically. A tool's maintainers can ask for its removal at any
time.

## Pull Request Process

1. Update the README.md with details of changes if needed
2. Update the documentation if you're changing functionality
3. The PR will be merged once you have the sign-off of at least one other developer
4. Make sure all tests pass and there are no merge conflicts

## Questions and Issues

If you have any questions or issues, please:
1. Check the existing issues to see if your question has already been answered
2. Create a new issue if needed
3. Be clear and descriptive in your issue

## License

By contributing to Zanadir, you agree that your contributions will be licensed under the project's license.

Thank you for contributing! 