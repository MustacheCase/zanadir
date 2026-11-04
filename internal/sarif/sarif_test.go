package sarif

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAt(t *testing.T) {
	assert.Nil(t, At("", 3), "nowhere to point")
	assert.Equal(t, "a%20b/c.yml", At("a b/c.yml", 3)[0].PhysicalLocation.ArtifactLocation.URI)
	assert.Equal(t, 3, At("c.yml", 3)[0].PhysicalLocation.Region.StartLine)
	assert.Equal(t, 1, At("c.yml", 0)[0].PhysicalLocation.Region.StartLine, "SARIF lines start at 1")
}

func TestWrite(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, Write(&b, Log{Schema: Schema, Version: Version, Runs: []Run{{Results: []Result{}}}}))
	var back map[string]any
	require.NoError(t, json.Unmarshal(b.Bytes(), &back))
	assert.Equal(t, "2.1.0", back["version"])
	assert.NotContains(t, b.String(), "automationDetails", "left out unless set")
	assert.Contains(t, b.String(), "\n  ", "indented")
}
