package rules

import (
	"regexp/syntax"
	"strings"
)

// requiredLiterals returns strings one of which every match contains,
// lowercased, or nil when the regex has no such set worth checking. A matcher
// skips the regex on text that holds none of them.
func requiredLiterals(pattern string) []string {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil
	}
	literals := required(re.Simplify())
	for _, literal := range literals {
		if len(literal) < 3 {
			return nil
		}
	}
	return literals
}

func required(re *syntax.Regexp) []string {
	switch re.Op {
	case syntax.OpLiteral:
		return []string{strings.ToLower(string(re.Rune))}
	// Simplify has already rewritten counted repeats into these.
	case syntax.OpCapture, syntax.OpPlus:
		return required(re.Sub[0])
	case syntax.OpConcat:
		var best []string
		for _, sub := range re.Sub {
			if set := required(sub); set != nil && (best == nil || shortest(set) > shortest(best)) {
				best = set
			}
		}
		return best
	case syntax.OpAlternate:
		var union []string
		for _, sub := range re.Sub {
			set := required(sub)
			if set == nil {
				return nil
			}
			union = append(union, set...)
		}
		return union
	}
	return nil
}

func shortest(set []string) int {
	n := len(set[0])
	for _, s := range set[1:] {
		n = min(n, len(s))
	}
	return n
}
