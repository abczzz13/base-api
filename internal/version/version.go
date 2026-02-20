package version

import "runtime/debug"

var buildVersion = "dev"

func GetVersion() string {
	if buildVersion != "" && buildVersion != "dev" {
		return buildVersion
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return buildVersion
	}

	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	return buildVersion
}
