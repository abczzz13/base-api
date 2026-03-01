package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

var (
	buildVersion = "dev"
	gitCommit    = "unknown"
	gitBranch    = "unknown"
	gitTag       = "unknown"
	buildTime    = "unknown"
	gitTreeState = "unknown"

	readBuildInfo = debug.ReadBuildInfo
)

type BuildMetadata struct {
	Version        string
	GitCommit      string
	GitCommitShort string
	GitBranch      string
	GitTag         string
	GitTreeState   string
	BuildTime      string
	GoVersion      string
}

func GetVersion() string {
	if gitTag != "" && gitTag != "unknown" {
		return gitTag
	}

	if buildVersion == "" || buildVersion == "dev" || buildVersion == "unknown" {
		if moduleVersion := getModuleVersion(); moduleVersion != "" {
			return moduleVersion
		}
	}

	if buildVersion == "" {
		return "dev"
	}

	return buildVersion
}

func getModuleVersion() string {
	info, ok := readBuildInfo()
	if !ok {
		return ""
	}

	if info.Main.Version == "" || info.Main.Version == "(devel)" {
		return ""
	}

	return info.Main.Version
}

func GetBuildInfo() string {
	metadata := GetBuildMetadata()

	return fmt.Sprintf(
		"Version: %s, Git: %s@%s, Branch: %s, Tree: %s, Built: %s, Go: %s",
		metadata.Version,
		metadata.GitCommitShort,
		metadata.GitTag,
		metadata.GitBranch,
		metadata.GitTreeState,
		metadata.BuildTime,
		metadata.GoVersion,
	)
}

func GetBuildMetadata() BuildMetadata {
	shortCommit := gitCommit
	if len(shortCommit) > 7 {
		shortCommit = shortCommit[:7]
	}

	return BuildMetadata{
		Version:        GetVersion(),
		GitCommit:      gitCommit,
		GitCommitShort: shortCommit,
		GitBranch:      gitBranch,
		GitTag:         gitTag,
		GitTreeState:   gitTreeState,
		BuildTime:      buildTime,
		GoVersion:      runtime.Version(),
	}
}
