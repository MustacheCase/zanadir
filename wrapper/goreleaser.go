package wrapper

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

var goreleaserFiles = []string{".goreleaser.yml", ".goreleaser.yaml", "goreleaser.yml", "goreleaser.yaml"}

type goreleaserSigning struct {
	Cmd  string   `yaml:"cmd"`
	Args []string `yaml:"args"`
}

// goreleaser follows a release into the SBOMs and signatures its config
// produces. Only a release does: build makes neither, and a snapshot ships
// nothing. sboms run syft and docker_signs cosign by default; signs and
// binary_signs default to gpg, which is not a supply chain control.
func (e *Expander) goreleaser(dir string, args []string) {
	file := ""
	var skip []string
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case (arg == "-f" || arg == "--config") && i+1 < len(args):
			i++
			file = args[i]
		case strings.HasPrefix(arg, "--config="):
			file = strings.TrimPrefix(arg, "--config=")
		case arg == "--snapshot" || i == 0 && !strings.HasPrefix(arg, "-") && arg != "release":
			return
		case strings.HasPrefix(arg, "--skip="):
			skip = append(skip, strings.Split(strings.TrimPrefix(arg, "--skip="), ",")...)
		}
	}
	if file == "" {
		file = findFile(dir, goreleaserFiles)
	}
	path, ok := e.within(dir, file)
	if file == "" || !ok || !e.once("goreleaser "+path) {
		return
	}

	var config struct {
		SBOMs       []goreleaserSigning `yaml:"sboms"`
		Signs       []goreleaserSigning `yaml:"signs"`
		BinarySigns []goreleaserSigning `yaml:"binary_signs"`
		DockerSigns []goreleaserSigning `yaml:"docker_signs"`
	}
	data, err := os.ReadFile(path)
	if err != nil || yaml.Unmarshal(data, &config) != nil {
		return
	}
	emit := func(entries []goreleaserSigning, cmd, kind string) {
		if slices.Contains(skip, kind) {
			return
		}
		for _, entry := range entries {
			e.emit(filepath.Dir(path), strings.Join(append([]string{cmp.Or(entry.Cmd, cmd)}, entry.Args...), " "))
		}
	}
	emit(config.SBOMs, "syft", "sbom")
	emit(config.Signs, "gpg", "sign")
	emit(config.BinarySigns, "gpg", "sign")
	emit(config.DockerSigns, "cosign", "docker-sign")
}
