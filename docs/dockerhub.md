# zanadir

**Does this repository's CI actually protect it?** zanadir reads a repository's CI configuration and reports which security and quality controls are missing, and which are wired so they cannot fail a build: a scanner with `continue-on-error`, a secret scan that only runs on a schedule, a quality gate that never aborts.

It reads **GitHub Actions, GitLab CI, Bitbucket Pipelines, Azure Pipelines, Jenkins and CircleCI**, and grades twelve categories: SCA, Secrets Detection, License Compliance, End Of Life, Coverage, Linter, Performance Testing, Unit Tests, SAST, IaC Security, Supply Chain and Workflow Security.

Source, documentation and issues: **https://github.com/MustacheCase/zanadir**

## Quick start

Scan any public GitHub repository - nothing to clone or mount:

```sh
docker run --rm mustachecase/zanadir scan --repo owner/name
```

Or mount a local checkout (including its `.git` directory):

```sh
docker run --rm -v "$PWD:/repo" mustachecase/zanadir scan --dir /repo
```

```
Coverage 9/11 - 2 categories need attention:
...
1 covered category is weakened:
  Secrets Detection is advisory (.github/workflows/ci.yml)
```

The image is published for `linux/amd64` and `linux/arm64`, so it runs natively on Apple Silicon and ARM runners.

## Common uses

```sh
# A private repository, a branch, tag or commit, or more than GitHub's anonymous rate limit
docker run --rm -e GITHUB_TOKEN mustachecase/zanadir scan --repo owner/name@v2.0.0

# Grade controls against the checks the default branch requires to merge
docker run --rm -e GITHUB_TOKEN mustachecase/zanadir scan --repo owner/name --required-checks

# Fail (exit 1) when a category has no tooling, or tooling that cannot fail a build
docker run --rm -v "$PWD:/repo" mustachecase/zanadir scan --dir /repo --enforce --fail-on-weakened

# Output for other tools: table (default), json (score, uncovered and weakened), sarif or markdown
docker run --rm -v "$PWD:/repo" mustachecase/zanadir scan --dir /repo -o sarif --output-file /repo/zanadir.sarif

# Print ready-to-paste CI configuration for what is missing
docker run --rm -v "$PWD:/repo" mustachecase/zanadir fix --dir /repo

# Keep settings and custom rules for in-house scanners in a file
docker run --rm -v "$PWD:/repo" mustachecase/zanadir scan --dir /repo -c /repo/.zanadir.yml
```

## How controls are graded

| Verdict | Meaning |
|---|---|
| `enforcing` | the control can fail the build on the path that gates a change |
| `partial` | it can fail a build, but not on that path - pushes only, say |
| `advisory` | it runs, and cannot block anything |
| `unknown` | the configuration cannot be read well enough to say |

## In GitHub Actions

Use the action rather than the image directly:

```yaml
- uses: mustachecase/zanadir-action@v1.26
  with:
    dir: .
```

See https://github.com/MustacheCase/zanadir-action.

## Tags

- `latest` - the newest release
- `0.20.0`, `0.19.0`, ... - one tag per [release](https://github.com/MustacheCase/zanadir/releases); pin one in CI

Every release has an SPDX SBOM attached on GitHub. The image is built `FROM scratch`: the static binary, CA certificates and the rule catalogue, nothing else.

## Licence

MIT.
