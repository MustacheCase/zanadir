package grading

import (
	"testing"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestByCategoryHoldsCodeQLToTheRepositoryLanguages(t *testing.T) {
	rs, err := rules.NewRulesService()
	require.NoError(t, err)
	missed := "CodeQL analyses only actions, python; the Java, Rust code detected from repository manifests is not scanned"
	for _, tt := range []struct {
		name, run string
		languages []string
		verdict   Verdict
		reason    string
	}{
		{"none of the code", "codeql database create --language=actions,python", []string{"Java", "Rust"}, Partial, missed},
		{"one of the code languages", "codeql database create --language=python", []string{"Python", "Rust"}, Enforcing, ""},
		{"an alias", "codeql database create --language=Java-Kotlin", []string{"Java"}, Enforcing, ""},
		{"every flag counts", "codeql database create --language go --language=rust", []string{"Rust"}, Enforcing, ""},
		{"a name zanadir can't detect", "codeql database create --language=c-cpp", []string{"Python"}, Enforcing, ""},
		{"an expression", "codeql database create --language=$LANGS", []string{"Python"}, Enforcing, ""},
		{"no flag", "codeql database create db", []string{"Rust"}, Enforcing, ""},
		{"no language CodeQL analyses", "codeql database create --language=python", []string{"PHP"}, Enforcing, ""},
		{"nothing detected", "codeql database create --language=python", nil, Enforcing, ""},
		{"only CodeQL's own commands", "lint --language=rust && codeql database create --language=python", []string{"Rust"}, Partial, "CodeQL analyses only python; the Rust code detected from repository manifests is not scanned"},
		{"another weakness first", "codeql database create --language=python || true", []string{"Rust"}, Advisory, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			artifact := &models.Artifact{
				Location:  "codeql.yml",
				Triggers:  []models.Trigger{{Event: "pull_request"}},
				Jobs:      []*models.Job{{Name: "analyze", Run: tt.run}},
				Languages: tt.languages,
			}
			sast := ByCategory(matcher.NewMatchService().Match([]*models.Artifact{artifact}, rs.GetCategoryRules(models.SAST)), nil)[string(models.SAST)]
			assert.Equal(t, tt.verdict, sast.Verdict)
			assert.Equal(t, tt.reason, sast.Reason)
		})
	}
}

func TestMissedCodeNeedsAStepOfARuleWithLanguages(t *testing.T) {
	rule := &rules.Rule{Languages: map[string][]string{"python": {"Python"}}}
	job := &models.Job{Run: "scan --language=python"}
	artifact := &models.Artifact{Languages: []string{"Rust"}}
	for _, f := range []*matcher.Finding{
		{Job: job, Artifact: artifact},
		{Rule: &rules.Rule{}, Job: job, Artifact: artifact},
		{Rule: rule, Artifact: artifact},
		{Rule: rule, Job: job},
	} {
		assert.Empty(t, missedCode(f))
	}
}
