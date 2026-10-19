package wrapper

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"gopkg.in/yaml.v3"
)

var precommitFiles = []string{".pre-commit-config.yaml", ".pre-commit-config.yml"}

// precommitValueFlags take a value, which is not a hook id.
var precommitValueFlags = map[string]bool{
	"--hook-stage": true, "--from-ref": true, "--to-ref": true, "-s": true, "-o": true, "--source": true,
	"--origin": true, "-c": true, "--config": true, "--remote-branch": true, "--local-branch": true,
	"--remote-name": true, "--remote-url": true, "--commit-msg-filename": true,
}

// precommitStageAliases are the stage names pre-commit renamed.
var precommitStageAliases = map[string]string{"commit": "pre-commit", "push": "pre-push", "merge-commit": "pre-merge-commit"}

type precommitHook struct {
	ID     string   `yaml:"id"`
	Entry  string   `yaml:"entry"`
	Args   []string `yaml:"args"`
	Stages []string `yaml:"stages"`
}

// precommit follows "pre-commit run" and "prek run" into the hooks they run:
// a hook from a hook repository by its id, which names its tool, a local one
// by its entry. Hook ids on the command line run only those hooks, and a hook
// limited to other stages than the one run is skipped.
func (e *Expander) precommit(dir string, args []string) {
	stage, file := "pre-commit", ""
	var only []string
	for i := 0; i < len(args) && args[i] != "--files"; i++ {
		flag, value, inline := strings.Cut(args[i], "=")
		switch {
		case !strings.HasPrefix(flag, "-"):
			only = append(only, args[i])
			continue
		case !precommitValueFlags[flag]:
			continue
		case !inline && i+1 < len(args):
			i++
			value = args[i]
		}
		switch flag {
		case "--hook-stage":
			stage = value
		case "-c", "--config":
			file = value
		}
	}
	if file == "" {
		file = findFile(dir, precommitFiles)
	}
	path, ok := e.within(dir, file)
	if file == "" || !ok || !e.once("pre-commit "+path+" "+stage+" "+strings.Join(only, " ")) {
		return
	}

	var config struct {
		DefaultStages []string `yaml:"default_stages"`
		Repos         []struct {
			Repo  string          `yaml:"repo"`
			Hooks []precommitHook `yaml:"hooks"`
		} `yaml:"repos"`
	}
	data, err := repofs.ReadFile(e.root, path)
	if err != nil || yaml.Unmarshal(data, &config) != nil {
		return
	}
	for _, repo := range config.Repos {
		for _, hook := range repo.Hooks {
			stages := hook.Stages
			if len(stages) == 0 {
				stages = config.DefaultStages
			}
			if len(only) > 0 && !slices.Contains(only, hook.ID) || !runsAt(stages, stage) {
				continue
			}
			command := hook.ID
			if repo.Repo == "local" && hook.Entry != "" {
				command = hook.Entry
			}
			e.emit(filepath.Dir(path), strings.Join(append([]string{command}, hook.Args...), " "))
		}
	}
}

func runsAt(stages []string, stage string) bool {
	canonical := func(s string) string { return cmp.Or(precommitStageAliases[s], s) }
	return len(stages) == 0 || slices.ContainsFunc(stages, func(s string) bool { return canonical(s) == canonical(stage) })
}
