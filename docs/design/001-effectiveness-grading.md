# Design: grading control effectiveness

**Status:** proposal — no implementation yet
**Scope:** how zanadir could report whether a control actually protects anything, not just whether it is present

## Problem

Today a category is binary: some rule matched, so the category is covered and no
suggestion is emitted. That is the entire model, in
[`suggester.go`](../../suggester/suggester.go):

```go
coveredCategories[f.Category] = true
```

Presence is not effectiveness. All of the following mark a category "covered"
while protecting nothing:

| Configuration | Why it does not protect |
|---|---|
| `continue-on-error: true` on the scanner step | The job stays green no matter what the scanner finds |
| `trivy fs --exit-code 0` | Findings are printed, never enforced |
| Scanner runs only on `schedule:` | Nothing is blocked at merge time |
| `if: github.ref == 'refs/heads/main'` | Pull requests are never scanned |
| Scanner runs only on `workflow_dispatch` | It runs when someone remembers |
| `soft_fail: true` (tfsec, several actions) | Same as `continue-on-error` |
| Step is in a job that is never referenced by a workflow trigger | Dead configuration |

This matters more than adding rules for more tools. A repository that has
adopted gitleaks and wired it up so it can never fail is in a *worse* position
than one with no gitleaks at all, because everyone believes the control exists.
Reporting "covered" there is an actively wrong answer.

It is also the most defensible thing zanadir could do. Detecting whether a
string appears in a workflow is easy to copy; modelling whether a control is
wired up to actually block a merge is not.

## Why it is not a small change

The parsers currently discard everything except the tool identity. After
[`parser/github.go`](../../parser/github.go) runs, an artifact is:

```go
type Artifact struct {
    Name     string
    Jobs     []*Job     // Name, Package, Version
    Location string
}
```

There is no representation of:

- workflow triggers (`on:`)
- branch filters
- `continue-on-error`, at either step or job level
- `if:` conditions
- step arguments (`with:`, and the flags inside a `run:` command)
- the job graph (`needs:`), so we cannot tell whether a job is reachable

Every signal in the table above lives in one of those. So the parser and model
work is the bulk of this, and the grading logic on top is comparatively small.

## Proposed model

Keep the binary "is the tool present" answer, and add an orthogonal
**effectiveness** verdict to each finding:

```
enforcing    the control can fail the build on the default path
advisory     the control runs but cannot fail the build
partial      the control can fail the build, but not on every relevant path
                (e.g. pushes only, not pull requests)
unreachable  the control is configured but never runs
```

A category is then reported along three lines rather than two:

- **not covered** — no tool at all (today's suggestion)
- **covered, but weakened** — a tool exists with a verdict below `enforcing`
- **covered** — at least one `enforcing` control

The second is new output, and is the whole point of the feature.

### Data model changes

```go
type Artifact struct {
    Name     string
    Triggers []Trigger  // new
    Jobs     []*Job
    Location string
}

type Trigger struct {
    Event    string   // push, pull_request, schedule, workflow_dispatch
    Branches []string // branch filter, empty means all
}

type Job struct {
    Name            string
    Package         string
    Version         string
    Run             string
    ContinueOnError bool     // new
    If              string   // new
    Needs           []string // new
    With            map[string]string // new
}
```

`Run`, from the shell-step work, already exists.

### Where the checks live

Two options, and the choice matters for how contributors extend this.

**Option A — extend the existing rule schema.** Add a `weakenedWhen` block to a
rule in `rules/storage/*.yaml`:

```yaml
- id: "gitleaks-rule"
  applyOn: ["Artifact.Name", "Job.Package", "Job.Run"]
  categories: ["Secrets Detection"]
  regex: "(?i)\\bgitleaks\\b"
  weakenedWhen:
    - field: "Job.ContinueOnError"
      equals: true
      verdict: "advisory"
    - field: "Job.Run"
      regex: "--exit-code\\s+0"
      verdict: "advisory"
```

Keeps everything about a tool in one place, and contributors already understand
this file. But it grows a small expression language inside YAML, which tends to
end badly.

**Option B — generic checks independent of the tool.** Most degradations are
tool-agnostic: `continue-on-error` weakens *any* control. Express those once,
in Go, and let rules opt out. Far less YAML, but tool-specific flags like
`--exit-code 0` still need somewhere to live.

**Recommendation: B for the generic signals, with a narrow `weakenedWhen` for
tool-specific flags.** Most of the value is in the generic set, and it should
not require touching 29 rule files.

## Interaction with existing features

- **Enforcement** (`--fail-on`, baseline): a weakened control should be able to
  fail a build. Probably a `--fail-on-weakened` flag, off by default, since
  turning weak controls into failures overnight would break people.
- **SARIF**: maps naturally. A weakened control is a `note`-level result;
  a missing one stays `warning`.
- **Baseline**: needs to record verdicts, not just category names, or accepting
  a gap would also silently accept a later degradation of it.

## Risks

- **False positives are costly here.** `continue-on-error: true` is legitimate
  while a team rolls a scanner out. Getting told your deliberate choice is wrong
  is more annoying than being told about a gap. Mitigation: an ignore
  annotation, and shipping the verdict as informational before it can fail
  anything.
- **The `if:` expression language is not fully analysable.** `if:` can reference
  arbitrary context. We should only reason about a small set of recognised
  patterns and report `unknown` otherwise, never guess.
- **Reusable workflows and composite actions hide configuration.** A `uses:`
  pointing at another repo is opaque. We can see the call, not what it does.
  Should be reported as `unknown` rather than assumed effective.

## Suggested phasing

1. **Capture the context.** Parser and model changes only, no grading, no
   output change. Verifiable in isolation and useful on its own.
2. **Generic verdicts.** `continue-on-error`, trigger analysis, branch filters.
   Surface in JSON and SARIF as informational.
3. **Tool-specific flags.** The narrow `weakenedWhen` escape hatch.
4. **Enforcement.** `--fail-on-weakened`, baseline records verdicts.

## Open questions

- Should a category with only advisory controls still produce a tool
  suggestion, or a different message ("you have gitleaks, wire it up")? The
  second is more useful and needs new output shape.
- Does `unreachable` need the full `needs:` graph, or is trigger analysis
  enough for a first version?
- How should the table output show verdicts without becoming unreadable?
