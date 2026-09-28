package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	t.Parallel()
	const revision = "f799367abcdef0123456789abcdef0123456789a"
	vcs := func(revision, modified string) []debug.BuildSetting {
		return []debug.BuildSetting{
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: revision},
			{Key: "vcs.time", Value: "2026-09-28T20:00:00Z"},
			{Key: "vcs.modified", Value: modified},
		}
	}
	info := func(version string, settings []debug.BuildSetting) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/mantas6/sat-cli", Version: version}, Settings: settings}
	}

	tests := []struct {
		name     string
		override string
		info     *debug.BuildInfo
		ok       bool
		want     string
	}{
		{name: "ldflags override wins", override: "1.2.3", info: info("v0.4.0", vcs(revision, "false")), ok: true, want: "1.2.3"},
		{name: "override without build info", override: "1.2.3", want: "1.2.3"},
		{name: "no build info", want: "dev"},
		{name: "nil build info", ok: true, want: "dev"},
		{name: "module version", info: info("v0.4.0", nil), ok: true, want: "v0.4.0"},
		{name: "pseudo-version", info: info("v0.0.0-20260928200000-f799367abcde+dirty", vcs(revision, "true")), ok: true, want: "v0.0.0-20260928200000-f799367abcde+dirty"},
		{name: "devel with clean revision", info: info("(devel)", vcs(revision, "false")), ok: true, want: "f799367abcde"},
		{name: "devel with modified revision", info: info("(devel)", vcs(revision, "true")), ok: true, want: "f799367abcde-dirty"},
		{name: "empty version with revision", info: info("", vcs(revision, "false")), ok: true, want: "f799367abcde"},
		{name: "short revision kept whole", info: info("(devel)", vcs("abc123", "false")), ok: true, want: "abc123"},
		{name: "devel without vcs", info: info("(devel)", nil), ok: true, want: "dev"},
		{name: "modified without revision", info: info("(devel)", vcs("", "true")), ok: true, want: "dev"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveVersion(test.override, test.info, test.ok); got != test.want {
				t.Fatalf("resolveVersion() = %q, want %q", got, test.want)
			}
		})
	}
}
