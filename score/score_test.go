package score

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/models"
)

func TestOfCountsEveryCategory(t *testing.T) {
	all := len(models.CategoryTitles)

	s := Of(nil, 3)
	if s.Total != all {
		t.Errorf("Total = %d, want %d", s.Total, all)
	}
	if s.Covered != all-3 {
		t.Errorf("Covered = %d, want %d", s.Covered, all-3)
	}
}

func TestOfDropsExcludedCategoriesFromTheDenominator(t *testing.T) {
	all := len(models.CategoryTitles)

	// The excluded category is not reported as uncovered either, so a
	// repository that excludes a gap scores full marks.
	s := Of([]string{string(models.Coverage)}, 0)
	if s.Total != all-1 {
		t.Errorf("Total = %d, want %d", s.Total, all-1)
	}
	if s.Percent() != 100 {
		t.Errorf("Percent = %d, want 100", s.Percent())
	}
}

func TestOfIgnoresRepeatedAndUnknownExclusions(t *testing.T) {
	all := len(models.CategoryTitles)

	s := Of([]string{"coverage", "Coverage", "not a category"}, 0)
	if s.Total != all-1 {
		t.Errorf("Total = %d, want %d", s.Total, all-1)
	}
}

func TestOfNeverGoesNegative(t *testing.T) {
	if got := Of(nil, len(models.CategoryTitles)+5).Covered; got != 0 {
		t.Errorf("Covered = %d, want 0", got)
	}
}

func TestScoreWithNothingApplicable(t *testing.T) {
	s := Of(models.CategoryNames(), 0)
	if s.Total != 0 {
		t.Fatalf("Total = %d, want 0", s.Total)
	}
	if s.String() != "n/a" {
		t.Errorf("String() = %q, want %q", s.String(), "n/a")
	}
	if s.colour() != "lightgrey" {
		t.Errorf("colour() = %q, want lightgrey", s.colour())
	}
}

func TestColourTracksCoverage(t *testing.T) {
	for _, tc := range []struct {
		s    Score
		want string
	}{
		{Score{Covered: 10, Total: 10}, "brightgreen"},
		{Score{Covered: 8, Total: 10}, "green"},
		{Score{Covered: 6, Total: 10}, "yellow"},
		{Score{Covered: 4, Total: 10}, "orange"},
		{Score{Covered: 3, Total: 10}, "red"},
		{Score{Covered: 0, Total: 10}, "red"},
	} {
		if got := tc.s.colour(); got != tc.want {
			t.Errorf("%s colour = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestWriteProducesAShieldsEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "badge.json")

	if err := Write(path, Score{Covered: 9, Total: 11}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("badge not written: %v", err)
	}

	var got endpoint
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("badge is not valid JSON: %v", err)
	}

	want := endpoint{SchemaVersion: 1, Label: "zanadir", Message: "9/11", Color: "green"}
	if got != want {
		t.Errorf("badge = %+v, want %+v", got, want)
	}
}

func TestWriteReportsAnUnwritablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "badge.json")

	err := Write(path, Score{Covered: 1, Total: 2})
	if err == nil {
		t.Fatal("expected an error for an unwritable destination")
	}
}
