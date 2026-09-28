package main

import "runtime/debug"

// version is set at build time with -ldflags "-X main.version=...". When it
// is empty, the version is derived from the binary's build information.
var version string

// shortRevisionLength matches the commit hash length of Go pseudo-versions.
const shortRevisionLength = 12

// resolveVersion returns override when set. Otherwise it returns the main
// module version recorded by the go command (for example the tag or
// pseudo-version from go install), then the short VCS revision with a
// "-dirty" suffix for modified checkouts, and finally "dev". info and ok are
// the results of debug.ReadBuildInfo.
func resolveVersion(override string, info *debug.BuildInfo, ok bool) string {
	if override != "" {
		return override
	}
	if !ok || info == nil {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}

	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return "dev"
	}
	if len(revision) > shortRevisionLength {
		revision = revision[:shortRevisionLength]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}
