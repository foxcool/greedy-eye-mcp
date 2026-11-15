package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestFormat(t *testing.T) {
	vcs := func(rev, dirty, tm string) *debug.BuildInfo {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: rev},
			{Key: "vcs.modified", Value: dirty},
			{Key: "vcs.time", Value: tm},
		}}
	}
	tests := []struct {
		name                  string
		version, commit, date string
		bi                    *debug.BuildInfo
		want                  string
	}{
		{
			name:    "injected values win over VCS data",
			version: "1.2.3", commit: "abcdef1234567890", date: "2026-01-02T03:04:05Z",
			bi:   vcs("ffffffffffff", "true", "2000-01-01T00:00:00Z"),
			want: "1.2.3 (commit abcdef123456, built 2026-01-02T03:04:05Z)",
		},
		{
			name: "no injection falls back to dev plus VCS revision",
			bi:   vcs("0123456789abcdef", "false", "2026-03-04T05:06:07Z"),
			want: "dev (commit 0123456789ab, built 2026-03-04T05:06:07Z)",
		},
		{
			name: "dirty tree is flagged",
			bi:   vcs("0123456789abcdef", "true", "2026-03-04T05:06:07Z"),
			want: "dev (commit 0123456789ab-dirty, built 2026-03-04T05:06:07Z)",
		},
		{
			name: "no injection and no VCS data",
			bi:   &debug.BuildInfo{},
			want: "dev (commit unknown, built unknown)",
		},
		{
			name: "no build info at all",
			want: "dev (commit unknown, built unknown)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := format(tt.version, tt.commit, tt.date, tt.bi); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
