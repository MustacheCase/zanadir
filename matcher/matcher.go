package matcher

import (
	"regexp"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
)

type Finding struct {
	Category string
	RuleID   string
	Location string
}

const (
	scriptLimit = 100
)

type Matcher interface {
	Match([]*models.Artifact, []*rules.Rule) []*Finding
}

type service struct{}

func (s *service) Match(artifacts []*models.Artifact, ruleSet []*rules.Rule) []*Finding {
	var findings []*Finding

	for _, rule := range ruleSet {
		for _, artifact := range artifacts {
			for _, applyField := range rule.ApplyOn {
				if matchesRule(artifact, rule, applyField) {
					for _, c := range rule.Categories {
						findings = append(findings, &Finding{
							Category: c,
							RuleID:   rule.ID,
							Location: artifact.Location,
						})
					}
				}
			}
		}
	}

	return findings
}

func matchesRule(artifact *models.Artifact, rule *rules.Rule, field string) bool {
	check := func(value string) bool {
		value = sanitizeScript(value)
		return value != "" && rule.Regex.MatchString(value)
	}

	switch field {
	case "Artifact.Name":
		return check(artifact.Name)
	case "Job.Package":
		return anyMatch(artifact.Jobs, func(job *models.Job) bool { return check(job.Package) })
	case "Job.Script":
		return anyMatch(artifact.Jobs, func(job *models.Job) bool { return check(job.Script) })
	}

	return false
}

// sanitizeScript removes potentially dangerous characters from the script.
func sanitizeScript(script string) string {
	// Example: Limit script length to 1000 characters.
	if len(script) > scriptLimit {
		script = script[:scriptLimit]
	}

	// Example:  Remove characters that could be used for regex injection.
	re := regexp.MustCompile(`[(){}\[\]\\.*+?]`) // Example: Remove regex meta characters
	script = re.ReplaceAllString(script, "")

	return script
}

// anyMatch checks if any element in a slice satisfies the predicate
func anyMatch[T any](items []T, predicate func(T) bool) bool {
	if items == nil {
		return false
	}

	for _, item := range items {
		if predicate(item) {
			return true
		}
	}
	return false
}

func NewMatchService() Matcher {
	return &service{}
}
