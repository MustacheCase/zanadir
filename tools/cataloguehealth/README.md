# Catalogue health

zanadir recommends tools (`suggester/suggestions.yaml`) and writes fixes that
pin actions (`fixer/templates.yaml`, and the workflow skeleton in
`fixer/fixer.go`). Nothing in the code notices when one of them dies, so this
checks them against GitHub:

```sh
GITHUB_TOKEN=$(gh auth token) go run ./tools/cataloguehealth
```

**Broken** - zanadir points users at something that is gone:

- the repository does not exist, or is archived;
- a fix pins an action to a ref that is not a tag.

**Out of date** - worth a look, not necessarily wrong:

- no release in a year. A release is the later of the latest GitHub release
  and the newest version tag, since many actions tag without publishing
  releases. A repository with neither is judged by its last push;
- a fix pins an older major version than the action's newest.

Recommendations that are not GitHub repositories are not checked.

The `Catalogue health` workflow runs this weekly with `-out report.md`, which
leaves the file empty when there is nothing to report. It keeps one issue
open, updated each week, while there are problems, and closes it once there
are none. Problems never fail the run, only being unable to check: a stale
catalogue is a decision for a maintainer, not a broken build.
