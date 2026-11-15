// Package buildinfo states which build of the server is running, so a stale
// binary shows up as a version string instead of as a silently missing tool.
package buildinfo

import "runtime/debug"

// Set with -ldflags "-X .../internal/buildinfo.Version=..." by the Makefile and
// the release build. A plain `go build` leaves them empty and String falls back
// to the VCS data Go embeds in the binary.
var (
	Version string
	Commit  string
	Date    string
)

const (
	unknown   = "unknown"
	shortSHA  = 12
	devMarker = "dev"
)

// String describes the running build, e.g. "dev (commit 0123456789ab-dirty,
// built 2026-03-04T05:06:07Z)". It is the single string used for the MCP server
// info version, the startup log line and the version tool.
func String() string {
	bi, _ := debug.ReadBuildInfo()
	return format(Version, Commit, Date, bi)
}

// format prefers injected values and fills gaps from bi. Without an injected
// version the build is "dev"; the commit then comes from VCS stamping, with a
// "-dirty" suffix for a modified tree, which is how a local build is told apart
// from a released one.
func format(version, commit, date string, bi *debug.BuildInfo) string {
	var vcsRev, vcsTime string
	var dirty bool
	if bi != nil {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				vcsRev = s.Value
			case "vcs.time":
				vcsTime = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
	}

	if version == "" {
		version = devMarker
	}
	if commit == "" {
		commit = vcsRev
		if commit != "" && dirty {
			commit = short(commit) + "-dirty"
			return assemble(version, commit, pick(date, vcsTime))
		}
	}
	return assemble(version, short(commit), pick(date, vcsTime))
}

func assemble(version, commit, date string) string {
	if commit == "" {
		commit = unknown
	}
	return version + " (commit " + commit + ", built " + date + ")"
}

func pick(injected, fallback string) string {
	if injected != "" {
		return injected
	}
	if fallback != "" {
		return fallback
	}
	return unknown
}

func short(sha string) string {
	if len(sha) > shortSHA {
		return sha[:shortSHA]
	}
	return sha
}
