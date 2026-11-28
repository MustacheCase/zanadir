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

- **The `improve-ci` skill** (`/zanadir:improve-ci`). Claude scans the
  repository, reads its CI files, and gives you a prioritised list of changes.
  It asks which ones to apply before editing any file. Afterwards it scans
  again and reports the result.
- **The zanadir MCP server**, with three read-only tools:
  - `scan` reports the uncovered and weakened categories as JSON;
  - `fix` prints ready-to-paste CI configuration for the uncovered categories;
  - `skills` checks the repository's agent skills (`SKILL.md` files).

  Each tool reads the open project by default, a `dir` inside it, or a public
  GitHub repository given as `repo`.

## Install

```
/plugin marketplace add MustacheCase/zanadir
/plugin install zanadir@mustachecase
```

The MCP server needs either a zanadir with the `mcp` command on your `PATH`
(see [Installation](https://github.com/MustacheCase/zanadir#installation)),
or Docker.

## What it runs

- **The MCP server** starts through `scripts/zanadir-mcp`, a short shell
  script. If a `zanadir` on your `PATH` has the `mcp` command, it runs
  `zanadir mcp --dir <project>`. Otherwise it runs the Docker image
  `mustachecase/zanadir`, pinned to a release by tag and digest in that
  script. The project is mounted read-only. If neither is available, it prints
  how to install zanadir and exits.
- **The skill** lets Claude run `zanadir scan`, `zanadir fix`,
  `zanadir skills`, `zanadir --help` and `zanadir --version`, and read files
  with Read, Glob and Grep. Any other command or edit needs your approval as
  usual.

## What it sends or fetches

- **Scanning a local project reads only files.** Nothing is uploaded, and
  zanadir has no telemetry.
- **The Docker fallback** pulls the pinned `mustachecase/zanadir` image from
  Docker Hub the first time it runs.
- **A `repo` given to an MCP tool** is read through the GitHub API, which
  needs no clone. The plugin's server makes these requests without a token,
  so it can read only public repositories. The files go to a temporary
  directory, which is removed after the scan.
- **When the skill runs the zanadir CLI**, `zanadir scan --repo ...` and
  `zanadir scan --required-checks` call the GitHub API too, and use
  `GITHUB_TOKEN` or `GH_TOKEN` from your environment if one is set.

## Security checks

Every pull request and push to main scans this plugin with two skill security
scanners, in the
[Agent Skill Security workflow](https://github.com/MustacheCase/zanadir/actions/workflows/SkillSecurity.yml):

- **[NVIDIA SkillSpector](https://github.com/NVIDIA/SkillSpector)**, static
  analysis only, over the whole plugin folder: the skill, the launcher script
  and the MCP configuration. The job fails on any finding or on an incomplete
  scan. One finding is suppressed: a known false positive that takes the
  first option of the launcher's Docker command for an unpinned image
  ([NVIDIA/SkillSpector#781](https://github.com/NVIDIA/SkillSpector/issues/781));
  the suppression and its reason are in `.github/skillspector-baseline.yaml`.
- **[Cisco skill-scanner](https://github.com/cisco-ai-defense/skill-scanner)**,
  which fails on any high-severity finding.

The skill's [skill card](skills/improve-ci/skill-card.md) lists its owner,
intended use, output, and known risks with their mitigations.

## License

MIT, like zanadir itself.
