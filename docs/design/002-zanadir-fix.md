# Design: `zanadir fix`

**Status:** proposal — no implementation yet
**Scope:** generating ready-to-use CI configuration for the gaps zanadir reports

## Problem

A scan ends with a list of things you should have. Acting on it means leaving
the tool, finding the action, working out where it goes in your workflow, and
getting the YAML right. That gap between "here is what is missing" and "here is
it fixed" is where most reports die.

zanadir already knows everything needed to close it: the category, the
suggested tool, its repository, the CI platform in use, and — after language
detection — the ecosystem. The missing piece is a template per tool.

## What it would look like

```console
$ zanadir fix --dir .
Secrets Detection is not covered. Add to .github/workflows/security.yml:

  - name: Detect secrets
    uses: gitleaks/gitleaks-action@v2
    env:
      GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

$ zanadir fix --dir . --write
Wrote .github/workflows/zanadir-suggested.yml (2 categories)
```

## The central design question: how invasive to be

Three options, in increasing order of usefulness and risk.

**1. Print only.** Emit snippets to stdout, never touch files. Trivial, safe,
and leaves the copy-paste step with the user. This is most of the value for
almost none of the risk.

**2. Write a new workflow file.** Generate a self-contained
`.github/workflows/zanadir-suggested.yml`. Never edits an existing file, so
formatting and comments elsewhere cannot be damaged; worst case is a file the
user deletes. Idempotent by construction — regenerate and overwrite.

**3. Edit existing workflows.** Insert steps into the user's own jobs. The most
useful and by far the most dangerous. Round-tripping YAML loses or reorders
comments unless handled very carefully, and getting it wrong means corrupting a
file that gates the user's deploys.

**Recommendation: ship 1, then 2. Treat 3 as out of scope** until the templates
have proven themselves. The marginal benefit over 2 is small and the failure
mode is severe.

Option 2 has a real ergonomic downside worth acknowledging: a separate workflow
file duplicates checkout and setup steps, and runs as its own job, which is
slower and noisier in the PR checks list than adding a step to an existing job.
That is the price of not editing user files, and it should be stated in the
output so nobody is surprised.

## Templates

Templates belong next to the catalogue they extend, keyed by tool and platform:

```yaml
# suggester/templates.yaml
templates:
  - tool: "Gitleaks"
    platform: "github"
    step: |
      - name: Detect secrets
        uses: gitleaks/gitleaks-action@v2
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  - tool: "Gitleaks"
    platform: "gitlab"
    step: |
      secret_detection:
        image: zricethezav/gitleaks
        script:
          - gitleaks detect --source . --verbose
```

Not every tool needs a template. `fix` should quietly skip tools it has no
template for rather than emitting something half-right, and the catalogue
should be allowed to stay partial indefinitely.

### Keying on tool name is fragile

`tool: "Gitleaks"` is a string that must match `suggestions.yaml` exactly. This
project has now been bitten three times by exactly that pattern — the category
ids, the `applyOn` selectors, and the `Job.Name` selector all silently matched
nothing. Whatever key is chosen, **validate at load time that every template
resolves to a real suggestion, and fail loudly if not.**

## Hard parts

**Action version pinning.** A template hardcoding `@v2` goes stale, and worse,
recommends an unpinned tag when security guidance increasingly says pin to a
SHA. Options: pin to major tags and accept staleness; pin to SHAs and accept a
maintenance burden; or resolve latest at runtime, which adds a network
dependency to a tool that currently has none. Pinning to major tags with a
documented refresh process is the pragmatic choice, but this deserves a
decision rather than a default.

**Which categories to fix.** Generating steps for all eight at once produces a
large unfamiliar workflow. `fix` should probably take the same `--fail-on`-style
category selection so users can adopt one at a time.

**Secrets and configuration.** Some tools need credentials (Snyk, FOSSA,
GitGuardian). Templates should emit the `env:` block referencing a secret and
say plainly which secret must be created, rather than producing something that
silently fails on first run.

**Choosing among suggested tools.** Each category suggests several. `fix` has to
pick one, and the first entry is an arbitrary default. Either take an explicit
choice (`--tool Trivy`) or emit the default with a note about the alternatives.

## Interaction with existing features

- **Language detection** already narrows the tool list, so `fix` inherits a
  sensible default for free.
- **Baseline**: `fix` should ignore categories accepted in the baseline — the
  user has said they do not want them.
- A natural follow-on is opening the change as a pull request, but that pulls in
  git and forge credentials. Out of scope here; `--write` plus the user's own
  commit is enough.

## Suggested phasing

1. Template file, loader, and load-time validation. No command yet.
2. `zanadir fix` printing snippets for uncovered categories.
3. `--write` generating a standalone workflow file.
4. Category and tool selection flags, baseline awareness.

## Open questions

- Should templates live in the binary (embedded, like `suggestions.yaml`) or be
  overridable from the repo? Embedded is simpler; overridable helps teams with
  house standards, which is arguably the more valuable version.
- Is a standalone workflow file actually acceptable to users, or does the extra
  job make it a non-starter and force option 3 sooner than we would like?
- Should `fix` be a subcommand or a flag on `scan`? A subcommand keeps `scan`
  read-only, which seems worth preserving.
