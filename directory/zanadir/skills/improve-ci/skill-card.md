# Skill Card

## Description

improve-ci reviews a repository's CI/CD pipeline with zanadir and helps the
user close the gaps it finds: categories of checks with no tooling, and checks
that are present but cannot fail a build.

This skill is ready for commercial and non-commercial use.

## Owner

The zanadir maintainers, [MustacheCase/zanadir](https://github.com/MustacheCase/zanadir).
Report problems in the repository's issues; report vulnerabilities as its
[security policy](https://github.com/MustacheCase/zanadir/security/policy) describes.

## License/Terms of Use

[MIT](https://github.com/MustacheCase/zanadir/blob/main/LICENSE)

## Use Case

Developers and platform engineers who want to know what their CI is missing,
whether its security checks can actually fail a build, and how to add
dependency scanning, secrets detection, SAST, linting, tests, coverage and
other checks to GitHub Actions, GitLab CI, CircleCI, Bitbucket Pipelines,
Azure Pipelines or Jenkins.

## Deployment Geography for Use

Global.

### Requirements / Dependencies

Requires API Key or External Credential: No

Credential Type(s): None. The skill scans the open project, which reads only
local files.

It needs the zanadir CLI on `PATH`.

Do not include secrets in prompts/logs/output; use least-privilege credentials; rotate keys as appropriate.

## Known Risks and Mitigations

Risk: The skill may propose CI changes that break a pipeline or a deploy gate.

Mitigation: It gives a prioritised list and asks which changes to apply
before editing any file. Every command and edit goes through Claude Code's
normal approval, and it scans again afterwards and reports the result.

Risk: A proposed CI step could install or run a third-party tool.

Mitigation: Steps are based on `zanadir fix` templates, which pin actions by
commit and download releases with checksum verification, rather than written
from memory. The user reviews each change before it is applied.

Risk: zanadir can misjudge a check, for example miss a tool it does not
recognise or call a working check weakened.

Mitigation: Every finding names its file and reason, and the skill reads the
CI files itself before recommending, so the user can verify each one.

Risk: The skill could write a secret into a CI file.

Mitigation: It is told never to write secrets into files, to reference CI
secrets by name and to tell the user which ones to create.

## References

- [zanadir README](https://github.com/MustacheCase/zanadir#readme)
- [Plugin README: what it runs, sends and fetches](../../README.md)
- [Agent Skill Security workflow](https://github.com/MustacheCase/zanadir/actions/workflows/SkillSecurity.yml): NVIDIA SkillSpector and Cisco skill-scanner on every pull request and push to main

## Skill Output

Output type(s): Analysis, and CI configuration edits the user approves.

Output format: A prioritised Markdown list of recommendations; edits to CI
files such as `.github/workflows/*.yml` or `.gitlab-ci.yml`.

Output parameters: Each recommendation names the category, the file and the
reason, and what would change.

Other properties: The skill pre-approves only Read, Grep, Glob and LS. Each
zanadir command and each edit needs the user's approval.

## Skill Version

Plugin `zanadir` 1.0.1, directory edition, from [MustacheCase/zanadir](https://github.com/MustacheCase/zanadir). The skill is not signed.

## Ethical Considerations

The skill changes CI that may gate releases, so a person should review each
change before it is merged. It reads only the repository it is pointed at
and sends nothing elsewhere; zanadir has no telemetry.
