# Design: the skills blade

**Status:** phases 1 (`zanadir skills`) and 2 (Agent Skill Security) implemented; 3-5 proposed
**Scope:** auditing a repository's (or a user's) agent skills: what is missing,
what is broken, and what will never trigger

## Problem

Agent skills (`SKILL.md` files for Claude Code, Codex and other agents) are
becoming part of a repository the way CI configuration is: checked in, shared
by a team, and rarely reviewed after the first commit. They fail quietly. A
skill with a vague description never triggers, two overlapping skills fight
over the same requests, a script a skill points to was deleted months ago, and
nobody notices because nothing errors.

Security scanners for skills already exist and are good:
[NVIDIA SkillSpector](https://github.com/nvidia/skillspector) scores risk and
malicious intent, and Sentry's skill-scanner audits permissions and scripts.
What nobody does is zanadir's job: **look at what you have, tell you what is
missing or weak, and hand you the fix.**

## Why a blade, not a new tool

zanadir's bottleneck is distribution, not features. A second tool starts from
zero stars and splits the same attention. A blade reuses what zanadir already
has: language detection, the suggestion catalogue and its weekly health check,
the output formats, `fix`, the MCP server and the action.

A **blade** is one kind of thing zanadir scans, with its own categories and
rules. Today there is one, CI. This adds a second, skills. The CI blade stays
the default; nothing about `zanadir scan` changes.

We should not build a generic blade framework up front. Two blades is not
enough to know the right abstraction. Ship skills as its own command sharing
the existing packages, and extract an interface only if a third blade appears.

## What it would look like

```console
$ zanadir skills --dir .
Found 6 skills in .claude/skills, plugins/acme/skills

Quality
  ✗ deploy-helper     description has no "use when"; it will rarely trigger
  ✗ db-migrate        links scripts/rollback.sh, which does not exist
  ! review, pr-review descriptions overlap 81%; one will shadow the other

Safety
  ✗ setup-env         script pipes curl into sh (scripts/install.sh:4)

Gaps
  Repository uses Terraform and GitLab CI; no skill covers either.
    suggested: <a Terraform skill from the catalogue>, zanadir improve-ci

Score 61/100

$ zanadir skills --home     # also ~/.claude/skills and installed plugins
```

## Where skills are found

With `--dir`: `.claude/skills/*/SKILL.md`, `.claude/commands/*.md`,
`.agents/skills/*/SKILL.md`, and `*/skills/*/SKILL.md` under any directory with
a `.claude-plugin/plugin.json`. With `--home`: the same under `~/.claude`,
`~/.codex` and installed plugins. `--repo` works as it does for CI.

Each location must be checked against real repositories and real agents before
release, not assumed. zanadir's GitLab support was dead for months because a
path convention was mocked rather than verified.

## Rules

Three categories, mirroring how the CI blade has categories.

### Quality (deterministic, the core of v1)

| Rule | Why it matters |
|---|---|
| Frontmatter missing or not valid YAML | The agent cannot load the skill |
| `name` missing, not lowercase-hyphenated, over 64 chars, or not matching its directory | Loaded under the wrong name or not at all |
| `description` missing, under ~40 chars, or over 1024 | Too short to trigger; too long is truncated |
| `description` says what, never when ("Use when…", "Use for…") | The single biggest reason skills never trigger |
| Two descriptions overlap heavily (token similarity) | One shadows the other |
| Link or path to a file in the skill that does not exist | Instructions point at nothing |
| `SKILL.md` over ~500 lines with no `references/` split | Burns context on every load |
| `allowed-tools` grants bare `Bash` | Every command runs unasked; compare improve-ci's scoped `Bash(zanadir scan:*)` |

### Safety (shallow on purpose)

A short list of patterns cheap to detect with few false positives: `curl | sh`
in bundled scripts, zero-width or bidirectional Unicode in any skill file,
instructions to skip approvals or disable sandboxing, and secrets committed in
skill files. This is not a security scanner and should say so: the report
recommends SkillSpector for a real audit, the way the CI blade recommends
tools rather than being them.

### Gaps (the zanadir part)

Repository signals map to skill topics, each with catalogue suggestions:

| Signal | Suggested topic |
|---|---|
| Terraform, Pulumi, Kubernetes manifests | IaC |
| A CI configuration | CI (zanadir's own improve-ci) |
| Database migrations directory | Migrations |
| A language from `language.Detect` with no testing skill | Testing for that language |
| Frontend framework | UI / browser testing |

Suggestions live in a new section of `suggestions.yaml`, so the weekly
catalogue health check covers them. Skill repositories rot faster than CI
tools, so this matters more here.

### And one rule for the CI blade

A new CI category, **Agent Skill Security**, applies only when the repository
contains skills: is SkillSpector or an equivalent run in CI on them? This is
the cheapest piece and works through the existing rule engine.

Phase 2 built it with three tools: Cisco Skill Scanner, SkillSpector and Snyk
Agent Scan. None of the 17 skill repositories used in phase 1 runs any of
them, and GitHub code search finds almost no CI file that does, so for now
nearly every repository with skills is uncovered. SkillSpector's own CI was
the one near-miss: it runs `skillspector --version` and builds an image of
that name, so the rule credits only its `scan` command. The fix snippet uses
Cisco's CLI, because it installs from PyPI at a pinned version and takes a
severity threshold; SkillSpector installs from git, and Snyk Agent Scan
covers MCP servers first.

## Fix

`zanadir skills fix` in the same three steps as `zanadir fix`, stopping at the
second: print, then write. It can generate a missing frontmatter field, rewrite
a description into "what + use when" form from the skill body as a template
(not an LLM: no paid services), and scaffold a skill directory for a gap. It
never edits a skill's body.

## What phase 1 learned

Phase 1 was tuned against 2,136 skills in 17 popular skill repositories
(anthropics/skills, getsentry/skills, trailofbits/skills, obra/superpowers,
ComposioHQ/awesome-claude-skills and others). It changed the design in five
ways:

- **Errors are only what is certainly broken.** Invalid YAML, a link to a file
  that does not exist, and text-hiding Unicode. Everything else is a warning,
  since agents tolerate a missing name or an over-long description in ways the
  specification does not say. The 148 broken links found were checked by hand
  and are real, mostly in skills copied between agent directories.
- **Safety rules read only what the agent follows or runs.** On reference
  documents they flagged security guides explaining `curl | sh`. Approval
  settings are read only in `SKILL.md` and shell scripts, because programs
  mention `bypassPermissions` mostly to check for it.
- **Secrets were dropped.** Every hit was a secret scanner's own patterns or
  test data, and the CI blade's Secrets Detection category already covers the
  repository.
- **Overlap compares skills one agent loads together**, those under the same
  `skills` directory, after removing words most descriptions in that directory
  share. Without that, one generated collection of 800 skills produced 267,000
  overlaps; with it, eight remain across the whole corpus, all plausible.
- **Zero-width characters are a warning, in `SKILL.md` only.** Real skills use
  them to escape Markdown fences; bidirectional overrides and tag characters
  stay errors anywhere.

`.claude/commands` files are not read yet, and `--repo` is not supported: a
fetched repository keeps the contents of CI files only, so supporting it means
keeping skill directories in `remote`.

## Phasing

1. **Quality and safety lint**, `--dir` only, table and JSON. Tuned against
   anthropics/skills, getsentry/skills and a few dozen repositories with
   skills until false positives are near zero.
2. **Agent Skill Security** category in the CI blade.
3. **Gaps**, with the catalogue and its health check.
4. **`--home`**, SARIF and Markdown, MCP tool and action input.
5. **`skills fix`**.

## Non-goals

- Deep security analysis or a malicious-skill verdict (SkillSpector does it).
- Running or evaluating skills, or judging prose with an LLM.
- A plugin marketplace, or ranking skills by popularity.

## Open questions

- Command name: `zanadir skills` or `zanadir scan --blade skills`? The first
  is easier to discover; the second leaves room for more blades.
- Should the skills score feed the CI score, or stay separate? Separate is
  simpler and avoids changing every existing badge.
- Overlap threshold: needs real data before picking a number.
