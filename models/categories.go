package models

import "strings"

const (
	SCA                CategoryTitle = "SCA"
	Secrets            CategoryTitle = "Secrets Detection"
	Licenses           CategoryTitle = "License Compliance"
	EndOfLife          CategoryTitle = "End Of Life"
	Coverage           CategoryTitle = "Coverage"
	Linter             CategoryTitle = "Linter"
	PerformanceTesting CategoryTitle = "Performance Testing"
	UnitTests          CategoryTitle = "Unit Tests"
	SAST               CategoryTitle = "SAST"
	IaC                CategoryTitle = "IaC Security"
	SupplyChain        CategoryTitle = "Supply Chain"
	Table              Format        = "table"
	JSON               Format        = "json"
)

var CategoryTitles = []CategoryTitle{SCA, Secrets, Licenses, EndOfLife, Coverage, Linter, PerformanceTesting, UnitTests, SAST, IaC, SupplyChain}

type CategoryTitle string

type Format string

// ResolveCategory returns the canonical CategoryTitle for a user-supplied name,
// matching case-insensitively. Callers that accept category names must resolve
// them here rather than comparing strings themselves, so an unrecognised name
// can be rejected instead of silently matching nothing.
func ResolveCategory(name string) (CategoryTitle, bool) {
	name = strings.TrimSpace(name)
	for _, title := range CategoryTitles {
		if strings.EqualFold(name, string(title)) {
			return title, true
		}
	}
	return "", false
}

// CategoryNames returns every valid category name, for error messages.
func CategoryNames() []string {
	names := make([]string, 0, len(CategoryTitles))
	for _, title := range CategoryTitles {
		names = append(names, string(title))
	}
	return names
}
