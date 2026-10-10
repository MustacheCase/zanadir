package rules

// ValidateRulesForTest exposes validateRules so the rules_test package can
// check the schema errors without being inside the package.
func ValidateRulesForTest(fileRules []FileRule) error {
	return validateRules(fileRules)
}
