# Plugin evals

An eval suite for [`claude plugin eval`](https://code.claude.com/docs/en/plugin-evals):
realistic requests run with the plugin and without it, so the difference shows
what the plugin adds.

| Case | The repository | A good answer |
|---|---|---|
| `trivy-reports-only` | Trivy scans dependencies on every pull request | says Trivy exits 0 on findings without `--exit-code`, so the scan can't fail the build |
| `swallowed-secrets-scan` | gitleaks runs behind `\|\| true` | puts the swallowed secrets scan ahead of the missing checks |
| `push-only-checks` | lint, tests and `npm audit` run on pushes to main only | says they run after merge and can't block a pull request |
| `gitlab-gaps` | GitLab CI with a linter and tests only | names the missing security checks, with GitLab configuration |
| `unrelated-request` | a rename in `app.py` | does the rename without the skill or a scan |

Each case's `scaffold.sh` builds its repository in the run's empty workspace.
The plugin's MCP tools answer from `mocks/zanadir/`, which `update-mocks.sh`
generates from the real `zanadir mcp` for each scaffold; rerun it after a
release that changes zanadir's output.

Run it from the repository root. `--scaffold` lets the cases' scaffold scripts
run:

```bash
claude plugin eval plugins/zanadir --scaffold
```

That is 5 cases × 3 runs × 2 arms, 30 agent runs plus the judge calls of the
`llm` graders, which count against your plan's usage limits or your API bill.
`--runs 1` makes a cheaper first pass. Results go to `evals/results/`, which
git ignores.
