package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/score"
	"github.com/MustacheCase/zanadir/suggester"
)

func orgSample() OrgReport {
	uncovered := func(ids ...string) []*suggester.CategorySuggestion {
		var out []*suggester.CategorySuggestion
		for _, id := range ids {
			out = append(out, &suggester.CategorySuggestion{ID: id, Name: id})
		}
		return out
	}
	return OrgReport{
		Org: "acme",
		Repositories: []OrgRepository{
			{Name: "acme/web", Report: Report{Score: score.Score{Covered: 10, Total: 12}, Platforms: []string{"github"},
				Suggestions: uncovered("SAST", "Supply Chain"), Weakened: []Weakness{{Category: "SCA", Verdict: "partial"}}}},
			{Name: "acme/api", Report: Report{Score: score.Score{Covered: 10, Total: 12}, Platforms: []string{"github"},
				Suggestions: uncovered("SAST", "Coverage")}},
			{Name: "acme/legacy", Report: Report{Score: score.Score{Covered: 0, Total: 11},
				Suggestions: uncovered("SCA", "Secrets Detection", "SAST", "Linter", "Unit Tests", "Coverage"), NotApplicable: []string{"IaC Security"}}},
		},
		Skipped: []OrgRepository{{Name: "acme/old", Reason: "archived"}},
		Failed:  []OrgRepository{{Name: "acme/huge", Reason: "archive too large"}},
	}
}

func TestOrgTableIsWorstFirst(t *testing.T) {
	var b strings.Builder
	if err := renderOrgTable(&b, orgSample()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"acme: 3 repositories scanned, 1 skipped, 1 could not be scanned\n",
		"no CI found",
		"SCA (partial)",
		"Could not be scanned:\n  huge: archive too large\n",
		"Skipped:\n  old: archived\n",
		"3 of 3", // SAST is uncovered everywhere
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	legacy, web, api := strings.Index(out, "| legacy"), strings.Index(out, "| web"), strings.Index(out, "| api")
	if !(legacy < web && web < api) {
		t.Errorf("want legacy (0/11), then web (10/12, weakened), then api: %d %d %d", legacy, web, api)
	}
	if strings.Contains(out, "acme/web") {
		t.Error("the organisation is in the headline, not on every row")
	}
}

func TestOrgTableBoundsItsLists(t *testing.T) {
	report := OrgReport{Org: "acme", Interrupted: true}
	for i := range maxTableRepositories + 2 {
		report.Repositories = append(report.Repositories, OrgRepository{Name: fmt.Sprintf("acme/r%02d", i),
			Report: Report{Score: score.Score{Covered: 11, Total: 12}, Platforms: []string{"github"}, Suggestions: []*suggester.CategorySuggestion{{ID: "SAST"}}}})
	}
	var b strings.Builder
	if err := renderOrgTable(&b, report); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"interrupted, so this covers only the repositories reached", "The 50 worst of 52 are shown", "47 more"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(out, "| r51 ") {
		t.Error("the table stops at the bound")
	}
}

// Nothing scanned leaves only the headline and the lists; a repository with
// nothing to score sorts last, as if fully covered.
func TestOrgTableWithLittleToShow(t *testing.T) {
	var b strings.Builder
	if err := renderOrgTable(&b, OrgReport{Org: "acme", Failed: []OrgRepository{{Name: "acme/a", Reason: "x"}}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "REPOSITORY") {
		t.Errorf("no table without repositories:\n%s", b.String())
	}
	sorted := worstFirst([]OrgRepository{{Name: "acme/none", Report: Report{}}, {Name: "acme/half", Report: Report{Score: score.Score{Covered: 6, Total: 12}}}})
	if sorted[0].Name != "acme/half" {
		t.Errorf("a repository with no total sorts as covered: %v", sorted)
	}
	b.Reset()
	if err := renderOrgTable(&b, OrgReport{Org: "acme", Repositories: []OrgRepository{{Name: "acme/all", Report: Report{Score: score.Score{Covered: 12, Total: 12}, Platforms: []string{"github"}}}}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "CATEGORY") {
		t.Errorf("no category table when nothing is missing:\n%s", b.String())
	}
}

func TestWriteOrgJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "org.json")
	report := orgSample()
	report.Format, report.DestPath = "json", path
	if err := WriteOrg(report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Organisation string `json:"organisation"`
		Scanned      int    `json:"scanned"`
		Repositories []struct {
			Repository string          `json:"repository"`
			Report     json.RawMessage `json:"report"`
		} `json:"repositories"`
		Categories []struct {
			Category      string `json:"category"`
			Uncovered     int    `json:"uncovered"`
			Weakened      int    `json:"weakened"`
			NotApplicable int    `json:"notApplicable"`
		} `json:"categories"`
		Skipped []struct{ Repository, Reason string } `json:"skipped"`
		Failed  []struct{ Repository, Reason string } `json:"failed"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Organisation != "acme" || decoded.Scanned != 3 || decoded.Repositories[0].Repository != "acme/legacy" {
		t.Errorf("unexpected head: %+v", decoded)
	}
	var single jsonReport
	if err := json.Unmarshal(decoded.Repositories[0].Report, &single); err != nil || single.Score.Total != 11 {
		t.Errorf("each entry is the single-repository report: %v %+v", err, single)
	}
	for _, c := range decoded.Categories {
		if c.Category == "SAST" && c.Uncovered != 3 || c.Category == "SCA" && (c.Uncovered != 1 || c.Weakened != 1) || c.Category == "IaC Security" && c.NotApplicable != 1 {
			t.Errorf("unexpected count: %+v", c)
		}
	}
	if len(decoded.Skipped) != 1 || len(decoded.Failed) != 1 || decoded.Failed[0].Reason != "archive too large" {
		t.Errorf("skipped and failed: %+v %+v", decoded.Skipped, decoded.Failed)
	}
}

func TestWriteOrgTableAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "org.txt")
	report := orgSample()
	report.DestPath = path
	if err := WriteOrg(report); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "acme: 3 repositories scanned") {
		t.Errorf("table not written: %s", data)
	}
	report.DestPath = filepath.Join(t.TempDir(), "missing", "org.txt")
	if err := WriteOrg(report); err == nil {
		t.Error("an unwritable destination is an error")
	}
	if err := renderOrgTable(failingWriter{err: os.ErrClosed}, report); err == nil {
		t.Error("a failing writer is an error")
	}
}
