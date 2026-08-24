package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/suggester"
	"github.com/stretchr/testify/assert"
)

func testSuggestions() []*suggester.CategorySuggestion {
	return []*suggester.CategorySuggestion{
		{
			ID:          "SCA",
			Name:        "SCA Open Source Tools",
			Description: "SCA tools help track open-source components.",
			Suggestions: []*suggester.Suggestion{
				{Name: "Trivy", Repository: "https://github.com/aquasecurity/trivy", Description: "A vulnerability scanner."},
				{Name: "Grype", Repository: "https://github.com/anchore/grype", Description: "Another scanner."},
			},
		},
		{
			ID:          "Coverage",
			Name:        "Coverage Tools",
			Description: "Coverage tools measure test completeness.",
		},
	}
}

func TestBuildSarifStructure(t *testing.T) {
	log := buildSarif(testSuggestions(), "")

	assert.Equal(t, sarifVersion, log.Version)
	assert.Equal(t, sarifSchema, log.Schema)
	assert.Len(t, log.Runs, 1)

	run := log.Runs[0]
	assert.Equal(t, toolName, run.Tool.Driver.Name)
	assert.Len(t, run.Tool.Driver.Rules, 2, "one rule per uncovered category")
	assert.Len(t, run.Results, 2, "one result per uncovered category")

	// Consumers drop results referencing an undeclared rule.
	declared := make(map[string]bool, len(run.Tool.Driver.Rules))
	for _, rule := range run.Tool.Driver.Rules {
		declared[rule.ID] = true
	}
	for _, result := range run.Results {
		assert.True(t, declared[result.RuleID], "result references undeclared rule %q", result.RuleID)
		assert.Equal(t, "warning", result.Level)
		assert.NotEmpty(t, result.Message.Text)
		assert.Equal(t, result.RuleID, result.PartialFingerprints["categoryId"])
	}
}

func TestSarifHelpListsTools(t *testing.T) {
	log := buildSarif(testSuggestions(), "")

	var scaHelp, coverageHelp string
	for _, rule := range log.Runs[0].Tool.Driver.Rules {
		switch rule.ID {
		case "SCA":
			scaHelp = rule.Help.Text
		case "Coverage":
			coverageHelp = rule.Help.Text
		}
	}

	assert.Contains(t, scaHelp, "Trivy")
	assert.Contains(t, scaHelp, "https://github.com/aquasecurity/trivy")
	assert.Contains(t, scaHelp, "Grype")

	// No tools means no tools section.
	assert.Equal(t, "Coverage tools measure test completeness.", coverageHelp)
	assert.NotContains(t, coverageHelp, "Suggested tools")
}

func TestSarifMessageMentionsTools(t *testing.T) {
	log := buildSarif(testSuggestions(), "")

	for _, result := range log.Runs[0].Results {
		if result.RuleID == "SCA" {
			assert.Contains(t, result.Message.Text, "Trivy, Grype")
		}
	}
}

func TestRenderSarifIsValidJSON(t *testing.T) {
	report, err := renderSarif(testSuggestions(), "")
	assert.NoError(t, err)

	var decoded map[string]interface{}
	assert.NoError(t, json.Unmarshal([]byte(report), &decoded))
	assert.Equal(t, "2.1.0", decoded["version"])
	assert.True(t, strings.HasPrefix(report, "{"))
}

// SARIF requires runs[].results, so it must serialise as [] rather than null.
func TestRenderSarifWithNoSuggestions(t *testing.T) {
	report, err := renderSarif(nil, "")
	assert.NoError(t, err)
	assert.Contains(t, report, `"results": []`)
	assert.Contains(t, report, `"rules": []`)

	var decoded map[string]interface{}
	assert.NoError(t, json.Unmarshal([]byte(report), &decoded))
}

func TestSarifResultsCarryALocation(t *testing.T) {
	log := buildSarif(testSuggestions(), ".github/workflows/ci.yml")

	assert.NotEmpty(t, log.Runs[0].Results)
	for _, result := range log.Runs[0].Results {
		assert.Len(t, result.Locations, 1, "result %q must carry a location", result.RuleID)
		phys := result.Locations[0].PhysicalLocation
		assert.Equal(t, ".github/workflows/ci.yml", phys.ArtifactLocation.URI)
		assert.Equal(t, 1, phys.Region.StartLine)
	}
}

func TestSarifOmitsLocationWithoutAnAnchor(t *testing.T) {
	log := buildSarif(testSuggestions(), "")

	for _, result := range log.Runs[0].Results {
		assert.Empty(t, result.Locations, "no anchor means no location rather than a wrong one")
	}
}

func TestRenderSarifSerialisesLocations(t *testing.T) {
	report, err := renderSarif(testSuggestions(), ".github/workflows/ci.yml")
	assert.NoError(t, err)

	var decoded struct {
		Runs []struct {
			Results []struct {
				Locations []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	assert.NoError(t, json.Unmarshal([]byte(report), &decoded))
	assert.Len(t, decoded.Runs[0].Results[0].Locations, 1)
	assert.Equal(t, ".github/workflows/ci.yml",
		decoded.Runs[0].Results[0].Locations[0].PhysicalLocation.ArtifactLocation.URI)
}
