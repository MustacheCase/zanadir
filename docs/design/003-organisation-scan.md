# Design: organisation-wide scan

**Status:** proposal — no implementation yet
**Scope:** scanning every repository of a GitHub organisation in one run, and reporting on them together

## Problem

zanadir answers one question about one repository: does its CI actually
protect it? The person who owns that question across a company does not ask it
one repository at a time. An AppSec or platform lead looks after tens or
hundreds of repositories. Their questions are about the set:

- Which of our repositories have no secret scanning at all?
- Where is a scanner present but wired so it cannot fail a build?
- Which repositories are worst, so we fix those first?
- Did it get better since last month?

Today they would script `zanadir scan --repo` over a list of repositories,
then merge a pile of JSON by hand. That's the work this feature removes. It's
also the buyer: a tool that answers for the whole organisation is one a team
adopts, rather than one an engineer runs once.

## What already exists

Most of the work is done; this design is mostly the loop and the report.

| Needed | Exists |
| --- | --- |
| Scan a repository without cloning | `--repo`, through `remote.FetchContext`: one tarball, configuration files only, a cancelled fetch cleans up |
| A verdict per category | `handler.analyse`: findings, grading, score, not-applicable categories |
| Machine-readable output per repository | JSON v2: `{score, platforms, uncovered, weakened, notApplicable, requiredChecks}` |
| Required checks | `--required-checks`, the `gate` package |
| Rate-limit handling against the GitHub API | `tools/stateofci` (retry on 403/429, `Retry-After`, back-off), not yet shared |

What's missing:

- listing an organisation's repositories;
- running the scan over them with bounded concurrency;
- surviving the repositories that fail;
- reporting across them.

## Proposed interface

```sh
zanadir scan --org github.com/acme
zanadir scan --org acme -o json --output-file acme.json
zanadir scan --org acme --fail-on "Secrets Detection" --fail-on-weakened
```

`--org` is a third target for `scan`, next to `--dir` and `--repo`, and
excludes both. The other flags keep their meaning, applied to every
repository: `--excluded-categories`, `--required-checks`, `--fail-on`,
`--enforce`, `--fail-on-weakened`, `--baseline`, `-o`, `--output-file`.
`fix` doesn't take `--org`, because generating workflows for a whole
organisation is a separate decision for each repository.

### Which repositories

`GET /orgs/{org}/repos?type=all`, paginated at 100. Private repositories are
included when the token can see them. Left out by default, each reported as
skipped with its reason:

- **archived:** nothing merges into it;
- **forks:** their CI is usually the upstream's;
- **empty** (`size: 0`): nothing to scan.

`--include-archived` and `--include-forks` bring them back. A repository with
no CI at all is **not** left out: it's the most important finding the report
can make.

`--org` takes an organisation. A user account (`/users/{user}/repos`) is a
small extension, left out of the first phase.

### A token is required

Each repository costs about three API requests: the repository, its tarball,
and sometimes its tree. With `--required-checks`, add a few more, plus one per
ruleset.
Without a token, GitHub allows 60 requests an hour, about 20 repositories, so
`--org` refuses to start without `GITHUB_TOKEN` rather than stall partway. With
one, the 5,000-an-hour limit covers more than 1,500 repositories an hour.

### Running the scans

- **Concurrency:** `--parallel N` repositories at a time, default 4. Each
  repository is fetched, analysed and removed before the next one starts in
  that worker, so disk use stays at N copies.
- **Rate limits:** a 403 or 429 is retried after `Retry-After`, or with a
  growing back-off, using the logic in `tools/stateofci`, moved into `remote`.
  If the limit outlasts a bound (say 15 minutes), the run stops and reports
  what it has.
- **Failures:** a repository that can't be scanned (too large, timeout, 404
  after listing) is recorded with its error, and the run goes on.
- **Progress:** one line per repository, written to stderr, so it doesn't mix
  into the report.

  ```
  [37/120] acme/payments: 6/12, 2 weakened
  ```

- **Cancellation:** SIGINT or SIGTERM stops new fetches, cleans up those in
  flight (the #312 mechanism), and prints the partial report.

### Per-repository configuration

A repository may keep a `.zanadir.yml`, with excluded categories (a CLI has no
use for Performance Testing) and custom rules. The organisation scan reads it
from the fetched copy, so each repository is graded as its own team grades it.
Flags on the command line apply on top, to every repository.
[Open question 1](#open-questions).

## The report

### Table and Markdown

Worst first: the lowest score, then the most weakened controls. Then the
category view, which answers "which repositories have no secret scanning".

```
acme: 120 repositories scanned, 4 skipped (archived), 2 failed

| Repository        | Score | Uncovered                                  | Weakened             |
|-------------------|-------|--------------------------------------------|----------------------|
| acme/legacy-api   | 0/11  | everything: no CI found                    |                      |
| acme/payments     | 4/12  | Secrets Detection, SCA, SAST, ... (8)      | Linter (advisory)    |
| acme/web          | 9/12  | Supply Chain, End Of Life, Performance     | SCA (partial)        |
| ...               |       |                                            |                      |

| Category          | Uncovered in | Weakened in | Repositories          |
|-------------------|--------------|-------------|-----------------------|
| Secrets Detection | 41 of 120    | 9           | legacy-api, payments, ... |
| SCA               | 18 of 120    | 22          | ...                   |

Failed: acme/monorepo (archive too large; clone it and scan with --dir), acme/old (404)
```

The Markdown form is the same, so it can go into a tracking issue, the way the
per-repository report goes into a pull request comment today.

### JSON

The per-repository object stays exactly the JSON v2 report, so anything that
reads `scan --repo -o json` can read each entry:

```json
{
  "organisation": "acme",
  "scanned": 120,
  "repositories": [
    {"repository": "acme/payments", "report": {"score": {...}, "platforms": [...], "uncovered": [...], "weakened": [...], "notApplicable": [...]}}
  ],
  "categories": [
    {"category": "Secrets Detection", "uncovered": 41, "weakened": 9, "notApplicable": 3}
  ],
  "skipped": [{"repository": "acme/old-fork", "reason": "fork"}],
  "failed": [{"repository": "acme/monorepo", "error": "..."}]
}
```

### CSV

One row per repository and category, `repository,category,status,verdict,location`,
for a spreadsheet or a BI tool. This is the CSV half of the roadmap's posture
export; the HTML half stays with that item.

### SARIF

Not offered for `--org`. Code scanning attaches SARIF to one repository, and
an organisation report has no single repository to attach to. A run per
repository, uploaded to each, is what `scan --repo` or the action already
does.

## Enforcement and baselines

The flags fail the run when any scanned repository fails them:

- **`--fail-on SAST`:** some repository has SAST uncovered;
- **`--enforce`:** some repository has any category uncovered;
- **`--fail-on-weakened`:** some repository has a weakened control.

The failure message names the repositories.

A baseline at organisation level is the per-repository baseline, keyed by
repository:

```yaml
# zanadir organisation baseline: gaps accepted per repository.
repositories:
  acme/legacy-api:
    uncovered: [Unit Tests, Coverage]
  acme/web:
    weakened:
      SCA: partial
```

`--write-baseline` writes it from a run, as it does for one repository. A
repository the baseline doesn't name has nothing accepted. That's how a lead
adopts zanadir across an organisation: accept today's state once, then fail on
anything new.

Repositories that failed to scan don't fail `--enforce`, which can't speak for
them, but the run reports them, and exits non-zero when every repository
failed. [Open question 3](#open-questions).

## Code changes

1. **`remote`:** `ListRepositories(ctx, org, token)` with pagination and the
   filters, plus the rate-limit retry moved from `tools/stateofci`, which then
   uses it.
2. **`handler`:** split `Execute` into an assessment that returns a value
   (score, uncovered, weakened, not applicable, gate), and the output and
   enforcement steps that use it. `--repo` and `--org` both call the
   assessment; only the way results are reported differs.
3. **`org` package:** the worker pool, fetching, assessing, cleaning up, and
   collecting results in a stable order (sorted by repository), so two runs
   over the same organisation give the same report.
4. **`output`:** the organisation table, Markdown, JSON and CSV.
5. **`baseline`:** the keyed format, read and written.
6. **`config`:** `--org`, `--parallel`, `--include-archived`,
   `--include-forks`, and their place in the config file.

None of this changes how a single repository is scanned or graded.

## Risks

- **Run time.** `--repo` takes 1–20 seconds per repository, most of it
  download. At 4 in parallel, 500 repositories take about 10–20 minutes. That
  suits a scheduled job, not a pre-merge check, and the docs should say so.
- **Rate limits** with `--required-checks`, which adds a few requests per
  repository. Bounded waiting and a partial report keep it predictable.
- **Large repositories** that GitHub won't archive already fail with "clone
  it and scan with --dir". In an organisation report they show up under
  failed, not as a crash.
- **Private data.** The report names private repositories and their gaps.
  It's the operator's own output, written where they choose, and nothing is
  sent anywhere. This differs from the State of CI report, which publishes
  aggregates only; this report stays local.
- **Report size.** At thousands of repositories the table gets long. JSON and
  CSV carry the detail, and the table could cap the repository list (worst 50)
  while the category view stays complete.

## Suggested phasing

1. **List and scan:** `--org`, the repository filters, the token check,
   `--parallel`, progress, failures, and the table and JSON reports. This
   alone answers "which repositories have no secret scanning".
2. **Enforce:** `--fail-on`, `--enforce` and `--fail-on-weakened` across the
   organisation, and per-repository `.zanadir.yml`.
3. **Adopt:** the organisation baseline and `--write-baseline`, plus Markdown
   and CSV.
4. **Deepen:** `--required-checks` at organisation scale with rate budgeting,
   user accounts, and the HTML export from the posture-export item.

## Not in scope

- A service, dashboard or database. zanadir stays a CLI; a scheduled CI job
  and the JSON or CSV it writes are the history.
- GitLab groups and other forges. `--repo` is GitHub-only today; a GitLab
  remote scan would come first.
- `fix` across an organisation.

## Open questions

1. **Per-repository `.zanadir.yml`:** should each repository's own exclusions
   and custom rules apply in an organisation scan, with the command line on
   top (proposed)? Or should one organisation-wide config apply to all, so the
   lead decides alone?
2. **`--org` on `scan`, or a new `zanadir org` command?** The proposal reuses
   `scan`, because every scan flag applies. A separate command would make room
   for organisation-only flags without crowding `scan --help`.
3. **Exit status when some repositories fail to scan:** proposed is to report
   them and fail only if all failed. Stricter would be to fail whenever any
   repository can't be read, since the report then can't vouch for it.
4. **Default filters:** are archived repositories and forks right to leave
   out by default?
