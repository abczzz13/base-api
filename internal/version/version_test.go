package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type metadataSnapshot struct {
	version         string
	gitCommit       string
	gitBranch       string
	gitTag          string
	buildTime       string
	gitTreeState    string
	buildInfoReader func() (*debug.BuildInfo, bool)
}

func snapshotMetadata() metadataSnapshot {
	return metadataSnapshot{
		version:         buildVersion,
		gitCommit:       gitCommit,
		gitBranch:       gitBranch,
		gitTag:          gitTag,
		buildTime:       buildTime,
		gitTreeState:    gitTreeState,
		buildInfoReader: readBuildInfo,
	}
}

func restoreMetadata(snapshot metadataSnapshot) {
	buildVersion = snapshot.version
	gitCommit = snapshot.gitCommit
	gitBranch = snapshot.gitBranch
	gitTag = snapshot.gitTag
	buildTime = snapshot.buildTime
	gitTreeState = snapshot.gitTreeState
	readBuildInfo = snapshot.buildInfoReader
}

func TestGetVersion(t *testing.T) {
	original := snapshotMetadata()
	t.Cleanup(func() {
		restoreMetadata(original)
	})

	tests := []struct {
		name          string
		version       string
		gitTag        string
		moduleVersion string
		moduleOK      bool
		want          string
	}{
		{
			name:    "returns tag when tag present",
			version: "1.2.3",
			gitTag:  "v1.2.3",
			want:    "v1.2.3",
		},
		{
			name:    "returns version when tag unknown",
			version: "1.2.3",
			gitTag:  "unknown",
			want:    "1.2.3",
		},
		{
			name:    "returns version when tag empty",
			version: "1.2.3",
			gitTag:  "",
			want:    "1.2.3",
		},
		{
			name:          "falls back to module version when version is default",
			version:       "dev",
			gitTag:        "unknown",
			moduleVersion: "v9.9.9",
			moduleOK:      true,
			want:          "v9.9.9",
		},
		{
			name:          "falls back to dev when version empty and no module info",
			version:       "",
			gitTag:        "unknown",
			moduleVersion: "",
			moduleOK:      false,
			want:          "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readBuildInfo = func() (*debug.BuildInfo, bool) {
				if !tt.moduleOK {
					return nil, false
				}

				return &debug.BuildInfo{Main: debug.Module{Version: tt.moduleVersion}}, true
			}

			buildVersion = tt.version
			gitTag = tt.gitTag

			got := GetVersion()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("GetVersion mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetBuildMetadata(t *testing.T) {
	original := snapshotMetadata()
	t.Cleanup(func() {
		restoreMetadata(original)
	})

	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return nil, false
	}

	buildVersion = "1.2.3"
	gitCommit = "abcdef123456"
	gitBranch = "main"
	gitTag = "v1.2.3"
	buildTime = "2026-02-27T12:34:56Z"
	gitTreeState = "clean"

	got := GetBuildMetadata()
	want := BuildMetadata{
		Version:        "v1.2.3",
		GitCommit:      "abcdef123456",
		GitCommitShort: "abcdef1",
		GitBranch:      "main",
		GitTag:         "v1.2.3",
		GitTreeState:   "clean",
		BuildTime:      "2026-02-27T12:34:56Z",
		GoVersion:      runtime.Version(),
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("GetBuildMetadata mismatch (-want +got):\n%s", diff)
	}
}

func TestGetBuildInfo(t *testing.T) {
	original := snapshotMetadata()
	t.Cleanup(func() {
		restoreMetadata(original)
	})

	tests := []struct {
		name         string
		version      string
		gitCommit    string
		gitBranch    string
		gitTag       string
		buildTime    string
		gitTreeState string
		want         string
	}{
		{
			name:         "includes tag as resolved version and short commit",
			version:      "1.2.3",
			gitCommit:    "abcdef123456",
			gitBranch:    "main",
			gitTag:       "v1.2.3",
			buildTime:    "2026-02-27T12:34:56Z",
			gitTreeState: "clean",
			want: fmt.Sprintf(
				"Version: v1.2.3, Git: abcdef1@v1.2.3, Branch: main, Tree: clean, Built: 2026-02-27T12:34:56Z, Go: %s",
				runtime.Version(),
			),
		},
		{
			name:         "falls back to version when tag unknown",
			version:      "dev",
			gitCommit:    "abc",
			gitBranch:    "feature/metadata",
			gitTag:       "unknown",
			buildTime:    "unknown",
			gitTreeState: "dirty",
			want: fmt.Sprintf(
				"Version: dev, Git: abc@unknown, Branch: feature/metadata, Tree: dirty, Built: unknown, Go: %s",
				runtime.Version(),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readBuildInfo = func() (*debug.BuildInfo, bool) {
				return nil, false
			}

			buildVersion = tt.version
			gitCommit = tt.gitCommit
			gitBranch = tt.gitBranch
			gitTag = tt.gitTag
			buildTime = tt.buildTime
			gitTreeState = tt.gitTreeState

			got := GetBuildInfo()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("GetBuildInfo mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
