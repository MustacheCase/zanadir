package app

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	oldVersion, oldRead := version, readBuildInfo
	t.Cleanup(func() { version, readBuildInfo = oldVersion, oldRead })
	built := func(v string, ok bool) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) { return &debug.BuildInfo{Main: debug.Module{Version: v}}, ok }
	}

	version, readBuildInfo = "0.15.0", built("v9.9.9", true)
	assert.Equal(t, "0.15.0", Version(), "the release build's version wins")
	version = ""
	assert.Equal(t, "v9.9.9", Version(), "go install records the module version")
	readBuildInfo = built("(devel)", true)
	assert.Equal(t, "dev", Version())
	readBuildInfo = built("", false)
	assert.Equal(t, "dev", Version())
}
