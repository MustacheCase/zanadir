package parser

import (
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/stretchr/testify/assert"
)

func TestDetect(t *testing.T) {
	tests := map[string]map[string]string{
		GitHub:    {".github/workflows/ci.yml": "on: push\n"},
		GitLab:    {".gitlab-ci.yml": "test:\n  script: [go test]\n"},
		CircleCI:  {".circleci/config.yml": "version: 2.1\n"},
		Bitbucket: {"bitbucket-pipelines.yml": "pipelines: {}\n"},
		Azure:     {"azure-pipelines.yaml": "steps: []\n"},
		Jenkins:   {"Jenkinsfile": "pipeline { agent any }\n"},
	}
	for id, files := range tests {
		t.Run(id, func(t *testing.T) {
			dir := testrepo.Write(t, files)
			assert.Equal(t, id, Detect(dir).ID)
			assert.True(t, Uses(dir, id))
		})
	}

	t.Run("defaults to github", func(t *testing.T) {
		assert.Equal(t, GitHub, Detect(t.TempDir()).ID)
	})
	t.Run("a workflows directory with no workflow in it", func(t *testing.T) {
		dir := testrepo.Write(t, map[string]string{".github/workflows/README.md": "x", ".gitlab-ci.yml": "a:\n  script: [x]\n"})
		assert.Equal(t, GitLab, Detect(dir).ID)
	})
}

func TestLookupAndUses(t *testing.T) {
	for _, p := range Platforms {
		found, ok := Lookup(p.ID)
		assert.True(t, ok)
		assert.Equal(t, p.Target, found.Target)
	}
	_, ok := Lookup("tekton")
	assert.False(t, ok)
	assert.False(t, Uses(t.TempDir(), "tekton"))
}
