package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const leaked = "leaked-from-outside"

// A repository under scan tries every way it has to make zanadir read a file
// beside it: .. in each kind of include, absolute paths, and symlinks to a
// file and to a directory. None of the outside files may reach a job (#23).
func TestAHostileRepositoryCannotReadOutsideItself(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	root := filepath.Join(base, "repo")
	write := func(dir, rel, content string) {
		path := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	link := func(target, rel string) {
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.Symlink(target, path))
	}

	workflow := "on: pull_request\njobs:\n  a:\n    runs-on: x\n    steps:\n      - run: " + leaked + "\n"
	for rel, content := range map[string]string{
		"Makefile":                      "lint:\n\t" + leaked + "\n",
		"extra.mk":                      "extra:\n\t" + leaked + "\n",
		"Taskfile.yml":                  "version: '3'\ntasks:\n  lint:\n    cmds: [" + leaked + "]\n",
		"package.json":                  `{"name": "outside", "scripts": {"lint": "` + leaked + `"}}`,
		"tox.ini":                       "[testenv:lint]\ncommands = " + leaked + "\n",
		"action/action.yml":             "runs:\n  using: composite\n  steps:\n    - run: " + leaked + "\n      shell: bash\n",
		"workflow.yml":                  workflow,
		"call.yml":                      "on: workflow_call\njobs:\n  a:\n    runs-on: x\n    steps:\n      - run: " + leaked + "\n",
		"gitlab.yml":                    "lint:\n  script: [" + leaked + "]\n",
		"pkg/package.json":              `{"name": "pkg", "scripts": {"lint": "` + leaked + `"}}`,
		".git/refs/remotes/origin/HEAD": "ref: refs/remotes/origin/main\n",
		"script.sh":                     "set -e\n" + leaked + "\n",
	} {
		write(outside, rel, content)
	}

	write(root, ".git/HEAD", "ref: refs/heads/main\n")
	write(root, ".github/workflows/ci.yml", `on: pull_request
jobs:
  a:
    runs-on: x
    steps:
      - run: make -f ../outside/Makefile lint
      - run: make -C ../outside lint
      - run: make lint
      - run: task -t ../outside/Taskfile.yml lint
      - run: task -d `+outside+` lint
      - run: npm --prefix ../outside run lint
      - run: cd ../outside && npm run lint
      - run: npm run lint --workspaces
      - run: tox -c ../outside/tox.ini -e lint
      - run: bash ../outside/script.sh
      - run: source `+outside+`/script.sh
      - run: ./linked-script.sh
      - run: bash -c "cd .. && bash outside/script.sh"
      - uses: ./../outside/action
      - uses: ./linked-action
  b:
    uses: ./../outside/call.yml
`)
	write(root, "Makefile", "include ../outside/extra.mk "+outside+"/extra.mk\nlint:\n\t$(MAKE) extra\n")
	write(root, "package.json", `{"workspaces": ["../outside/*", "/`+strings.TrimPrefix(outside, "/")+`/*", "linked/*"], "scripts": {"lint": "true"}}`)
	write(root, ".gitlab-ci.yml", "include:\n  - local: '../outside/gitlab.yml'\n  - local: '/../outside/gitlab.yml'\n  - '"+outside+"/gitlab.yml'\n")
	link(filepath.Join(outside, "workflow.yml"), ".github/workflows/linked.yml")
	link(filepath.Join(outside, "action"), "linked-action")
	link(outside, "linked/out")
	link(filepath.Join(outside, "Taskfile.yml"), "Taskfile.yml")
	link(filepath.Join(outside, "tox.ini"), "tox.ini")
	link(filepath.Join(outside, "script.sh"), "linked-script.sh")

	artifacts, err := NewRepositoryScanner().Scan(root)
	require.NoError(t, err)
	require.NotEmpty(t, artifacts)
	for _, artifact := range artifacts {
		for _, job := range artifact.Jobs {
			if strings.Contains(job.Run, leaked) || strings.Contains(job.Package, leaked) {
				t.Errorf("%s job %s read a file outside the repository:\n%s", artifact.Location, job.Name, job.Run)
			}
		}
	}
}
