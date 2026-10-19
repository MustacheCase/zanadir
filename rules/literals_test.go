package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequiredLiterals(t *testing.T) {
	for pattern, want := range map[string][]string{
		`(?i)\bgitleaks\b`:                     {"gitleaks"},
		`(?i)\bcheckov\b|bridgecrewio/checkov`: {"checkov", "bridgecrewio/checkov"},
		`(?i)(?:^|[\s/])grype(?:[\s:@/]|$)`:    {"grype"},
		`(?i)\b(npm|yarn|pnpm)\s+audit\b`:      {"audit"},
		`(?i)\bmvnw?\s[^\n]*\btest\b`:          {"test"},
		`(?:abc)+x`:                            {"abc"},
		`a{2,}bcd`:                             {"bcd"},
		`(?i)\bab\b`:                           nil,
		`(?i)\btests?\b|^x$`:                   nil,
		`[a-z]+`:                               nil,
		`(?:gitleaks)?`:                        nil,
		`(`:                                    nil,
	} {
		assert.Equal(t, want, requiredLiterals(pattern), pattern)
	}
}
