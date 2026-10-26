package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrintable(t *testing.T) {
	assert.Equal(t, ".github/workflows/ci one.yml", Printable(".github/workflows/ci one.yml"))
	assert.Equal(t, `a\u001b]0;x\u0007\u001b[2Jb.yml`, Printable("a\x1b]0;x\x07\x1b[2Jb.yml"))
	assert.Equal(t, `a\u202eb`, Printable("a\u202eb"), "a bidirectional override is escaped too")
}
