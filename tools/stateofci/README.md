# State of CI security

Scans the most-starred GitHub repositories that have CI with zanadir and
aggregates what it finds. Only the aggregates are published; the
per-repository results it keeps locally name projects and are never committed.

```sh
go build -o zanadir .
GITHUB_TOKEN=... go run ./tools/stateofci -zanadir ./zanadir -n 1000
```

Every stage resumes, so an interrupted run picks up where it stopped:

1. **Candidates** - twice `-n` of the most-starred repositories, not archived
   and not forks, from GitHub search (`.stateofci/candidates.json`). Many of
   the most-starred are lists, books or docs with no CI.
2. **Scan** - `zanadir scan --repo <name> -o json` for each, in star order,
   until `-n` have CI (`.stateofci/raw/`). Rate-limited requests are retried.
3. **Summary** - `.stateofci/summary.md` and `summary.json`: per category,
   how often it is covered where it applies, and how often covered tooling
   cannot fail the build.

## What the numbers mean

- **Covered** means zanadir found a tool for the category in the repository's
  CI configuration or dependency-bot configuration.
- **Cannot fail the build** counts pipeline tools graded `advisory` or
  `partial`: `continue-on-error`, `allow_failure`, schedule-only, push-only,
  `catchError`, tool flags that never fail. `unknown` is not counted.
- **Only a dependency bot** is a category whose strongest control is
  Dependabot or Renovate. They open pull requests rather than fail builds by
  design, so they are counted apart rather than as weak pipelines.

## What zanadir cannot see

zanadir reads files in the repository. Security features switched on in
repository settings leave no file: GitHub's secret scanning and push
protection, CodeQL default setup, Dependabot alerts without a
`dependabot.yml`. Many popular repositories use them, so Secrets Detection,
SAST and SCA coverage here is a floor for what those projects run, not a
census. The report measures what their CI configuration enforces.
