package app

import "runtime/debug"

// version is set at release time with -ldflags "-X github.com/MustacheCase/zanadir/app.version=...".
var version string

var readBuildInfo = debug.ReadBuildInfo

func Version() string {
	if version != "" {
		return version
	}
	if info, ok := readBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
