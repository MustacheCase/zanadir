package wrapper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const goreleaserConfig = `version: 2
sboms:
  - artifacts: archive
  - cmd: ../.tool/syft
    args: ["$artifact", "--output", "spdx-json=$document"]
signs:
  - cmd: .tool/cosign
    args: ["sign-blob", "--bundle=${signature}", "${artifact}"]
  - artifacts: checksum
binary_signs:
  - cmd: cosign
docker_signs:
  - artifacts: manifests
`

func TestGoreleaserFollowsTheConfig(t *testing.T) {
	files := map[string]string{".goreleaser.yaml": goreleaserConfig, "release/custom.yml": "docker_signs:\n  - cmd: notation\n"}
	assert.Equal(t, []string{
		"syft",
		"../.tool/syft $artifact --output spdx-json=$document",
		".tool/cosign sign-blob --bundle=${signature} ${artifact}",
		"gpg",
		"cosign",
		"cosign",
	}, expand(t, files, "goreleaser release --clean"))
	assert.Equal(t, []string{"syft", "../.tool/syft $artifact --output spdx-json=$document", "cosign"},
		expand(t, files, "goreleaser release --skip=sign,validate"), "--skip drops what it names")
	assert.Equal(t, []string{"notation"}, expand(t, files, "goreleaser release -f release/custom.yml"))
	assert.Equal(t, []string{"notation"}, expand(t, files, "goreleaser --config=release/custom.yml"))
	assert.Len(t, expand(t, files, "goreleaser"), 6, "release is the default command")
	assert.Len(t, expand(t, files, "goreleaser release && goreleaser release"), 6, "a config is followed once per step")
}

func TestGoreleaserFollowsOnlyWhatShips(t *testing.T) {
	files := map[string]string{".goreleaser.yml": goreleaserConfig}
	for _, script := range []string{
		"goreleaser release --snapshot --clean",
		"goreleaser build --single-target",
		"goreleaser check",
		"goreleaser release -f ../outside.yml",
	} {
		assert.Nil(t, expand(t, files, script), script)
	}
	assert.Nil(t, expand(t, map[string]string{}, "goreleaser release"), "no config")
	assert.Nil(t, expand(t, map[string]string{".goreleaser.yml": "sboms: {not: a list}"}, "goreleaser release"))
}

func TestGoMakeReleasesThroughGoreleaser(t *testing.T) {
	files := gomakeRepo()
	files[".make/main.go"] = "package main\n\nfunc main() { Makefile(goreleaser.Tasks()) }\n"
	files[".goreleaser.yaml"] = "signs:\n  - cmd: cosign\n"
	assert.Equal(t, []string{"go run -C .make . ci-release", "goreleaser release --clean", "cosign"}, expand(t, files, "make ci-release"))
}
