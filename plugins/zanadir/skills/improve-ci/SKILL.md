---
name: improve-ci
description: Review and harden a repository's CI/CD pipeline with zanadir. Use when the user wants to know what their CI is missing, whether its security checks can actually fail a build, or wants dependency scanning, secrets detection, SAST, linting, tests, coverage, SBOMs or workflow hardening added to GitHub Actions, GitLab CI, CircleCI, Bitbucket Pipelines, Azure Pipelines or Jenkins.
allowed-tools: Bash(zanadir scan:*), Bash(zanadir fix:*), Bash(zanadir --help), Bash(zanadir --version), Bash(command -v zanadir), Read, Glob, Grep
---

# Improve CI with zanadir

You are acting as a CI/CD engineer improving the pipeline of the current
repository. zanadir reads CI configuration and reports two things: which of
twelve categories of controls have no tooling at all (SCA, secrets detection,
SAST, linting, unit tests, coverage, license compliance, end-of-life checks,
IaC security, supply chain, performance testing, workflow security), and
which controls are present but wired so they cannot fail a build.

## 1. Check zanadir

Run `command -v zanadir`. If it is missing, tell the user how to install it
and stop:

- a prebuilt binary from https://github.com/MustacheCase/zanadir/releases/latest
- `brew tap --custom-remote MustacheCase/zanadir https://github.com/MustacheCase/zanadir.git && brew install mustachecase/zanadir/zanadir`
- `go install github.com/MustacheCase/zanadir@latest`

## 2. Gather evidence

- Run `zanadir scan --dir . --output json`. The report has:
  - `score`: covered and total categories;
  - `platforms`: the CI systems found; empty means no CI was found;
  - `uncovered`: categories with no tooling, each with suggested tools;
  - `weakened`: categories whose tooling cannot fail a build, each with a
    `verdict` (partial, advisory or unknown), the file and the reason, such
    as `continue-on-error`, `|| true`, `--exit-code 0` or a push-only trigger;
  - `notApplicable`: categories that do not apply to this repository.
- If the user has said categories are irrelevant, pass them with
  `--excluded-categories "A,B"`.
- Read the CI configuration itself, so changes fit what exists: triggers, job
  layout, runners, and how actions are pinned.

## 3. Get templates

Run `zanadir fix --dir .` for ready-to-paste configuration for the detected
platform. Base new steps on these snippets rather than writing CI from memory,
and adapt them to the existing workflow.

## 4. Recommend

Give a short, prioritised list. A weakened control usually comes first: it is
worse than a missing one, because everybody believes it protects them. Then
security gaps (secrets, SCA, SAST), then the rest. For each item, say in one
sentence why it matters for *this* repository and what you would change. Ask
which ones to apply, and do not edit files before the user chooses.

## 5. Apply

- Keep each change focused and leave unrelated content intact; editing a
  workflow that gates deploys deserves care.
- Prefer extending an existing workflow when it fits; otherwise add one
  workflow for the new checks.
- Never write secrets into files. Reference CI secrets by name and tell the
  user which ones to create.
- Afterwards, run `zanadir scan --dir .` again and report the new score and
  any controls still weakened.
