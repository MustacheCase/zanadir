# Contributing to Zanadir

Thank you for your interest in contributing to Zanadir! This document provides guidelines and instructions for contributing to this project.

## Prerequisites

- Go 1.24 or later
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