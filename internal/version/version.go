// Package version gives the version of the godoist program.
package version

import (
	"runtime/debug"
	"strings"
)

// Version is the release version, without "v". The release script sets it with
// -ldflags "-X github.com/biomassa/godoist/internal/version.Version=0.1.0".
var Version = ""

// String returns the version. Without a release version, it is the module version that
// Go writes into the program: "0.1.0" for "go install ...@v0.1.0", or a pseudo-version
// from Git for a local build (for example "0.1.1-0.20260927083354-55c949e7c378+dirty").
// "go run" gives no version: then String returns "dev".
func String() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return "dev"
}
