package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAliasExpansionIsCapped(t *testing.T) {
	defer func(limit int) { maxYAMLNodes = limit }(maxYAMLNodes)
	maxYAMLNodes = 100
	bomb := "a: &a [x, x, x, x, x]\nb: &b [*a, *a, *a, *a, *a]\nc: &c [*b, *b, *b, *b, *b]\nd: [*c, *c, *c, *c, *c]\n"

	var out map[string]any
	assert.ErrorContains(t, decodeYAML([]byte(bomb), &out), "expands to more than 100 nodes")
	assert.NoError(t, decodeYAML([]byte("a: &a [x]\nb: *a\nc: {<<: {k: v}}\n"), &out), "ordinary aliases and merge keys still decode")
}
