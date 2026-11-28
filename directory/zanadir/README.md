# zanadir plugin for Claude Code

[zanadir](https://github.com/MustacheCase/zanadir) reads a repository's CI
configuration (GitHub Actions, GitLab CI, CircleCI, Bitbucket Pipelines, Azure
Pipelines, Jenkins) and reports which categories of checks it has no tooling
for, such as dependency scanning, secrets detection, SAST, linting, tests and
coverage. It also reports which checks are present but wired so they can't
fail a build, for example behind `continue-on-error`, `|| true` or
`--exit-code 0`. This plugin brings it into Claude Code, so Claude can review
your CI and help close the gaps.

## What's in it

**The `improve-ci` skill** (`/zanadir:improve-ci`). Claude scans the
repository with zanadir, reads its CI files, and gives you a prioritised list
of changes. It asks which ones to apply before editing any file. Afterwards it
scans again and reports the result.

The skill asks to run the zanadir CLI in your project (`zanadir scan`,
`zanadir fix`, `zanadir skills`), each command with your approval. The plugin
itself contains no scripts, hooks or MCP servers, and nothing in it runs on its
own.

## Requirements

The zanadir CLI on your `PATH`. See
[Installation](https://github.com/MustacheCase/zanadir#installation) for
release binaries and package managers.

## What it runs

The skill runs `command -v zanadir`, `zanadir scan`, `zanadir fix` and
`zanadir skills` in your project, and reads files with Read, Glob, Grep and
LS. Claude Code asks your approval before each command and each edit; the
skill pre-approves only reading files.

The plugin contains no scripts, hooks or MCP servers.

## What it sends or fetches

**Scanning a local project reads only files.** Nothing is uploaded, and
zanadir has no telemetry. The skill scans the open project with `--dir .` and
doesn't ask zanadir to read anything over the network.

## License

MIT, like zanadir itself.
