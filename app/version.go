package app

import "runtime/debug"

// version is set at release time with -ldflags "-X github.com/MustacheCase/zanadir/app.version=...".
var version string

var readBuildInfo = debug.ReadBuildInfo

// Version reports the release zanadir was built as, else the module version go install recorded, else "dev".
func Version() string {
	if version != "" {
		return version
	}
	if info, ok := readBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
