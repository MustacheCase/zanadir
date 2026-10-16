package parser

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// The configuration zanadir reads was written by someone else, so these
// helpers never fail: a value they cannot read is skipped, and one malformed
// entry must not abort the scan of a whole repository.

// resolve follows YAML aliases to the node they name.
func resolve(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

// scalarIs reports whether node is a scalar equal to value, ignoring case.
func scalarIs(node *yaml.Node, value string) bool {
	node = resolve(node)
	return node.Kind == yaml.ScalarNode && strings.EqualFold(node.Value, value)
}

// scalars keeps the scalar values of a mapping, such as a task's inputs or a
// pipe's variables, which rules compare as text.
func scalars(m map[string]yaml.Node) map[string]string {
	if len(m) == 0 {
		return nil
	}
	values := make(map[string]string, len(m))
	for key, value := range m {
		if v := resolve(&value); v.Kind == yaml.ScalarNode {
			values[key] = v.Value
		}
	}
	return values
}

// stringList accepts either a scalar or a sequence, which "needs" and the
// branch filters both allow.
type stringList []string

func (s *stringList) UnmarshalYAML(node *yaml.Node) error {
	*s = decodeStrings(node)
	return nil
}

// decodeStrings reads a scalar or a sequence of scalars, skipping whatever it
// cannot read rather than discarding the entries beside it.
func decodeStrings(node *yaml.Node) []string {
	switch node.Kind {
	case yaml.ScalarNode:
		var one string
		if node.Decode(&one) != nil || one == "" {
			return nil
		}
		return []string{one}

	case yaml.SequenceNode:
		values := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			var one string
			if item.Decode(&one) == nil && one != "" {
				values = append(values, one)
			}
		}
		if len(values) == 0 {
			return nil
		}
		return values
	}
	return nil
}
