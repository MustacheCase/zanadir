package rules_test

import (
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
)

func TestGetCategoryRules(t *testing.T) {
	// Initialize the service using the embedded rules.
	rs, err := rules.NewRulesService()
	if err != nil {
		t.Fatalf("failed to initialize rules service: %v", err)
	}

	// Change "testCategory" to a category that exists in your embedded JSON file.
	result := rs.GetCategoryRules(models.CategoryTitle("SCA"))
	if len(result) == 0 {
		t.Fatalf("expected at least one rule in category 'testCategory'")
	}
}
