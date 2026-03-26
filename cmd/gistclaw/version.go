package main

import (
	"fmt"
	"io"
	"runtime/debug"
	"strings"
)

var (
	version   string
	commit    string
	buildDate string

	versionBuildInfo = loadBuildInfo
)

type buildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

func runVersion(stdout, _ io.Writer) int {
	info := resolvedBuildInfo()
	fmt.Fprintf(stdout, "version: %s\ncommit: %s\nbuild_date: %s\n", info.Version, info.Commit, info.BuildDate)
	return 0
}

func resolvedBuildInfo() buildInfo {
	info := versionBuildInfo()
	if trimmed := strings.TrimSpace(version); trimmed != "" {
		info.Version = trimmed
	}
	if trimmed := strings.TrimSpace(commit); trimmed != "" {
		info.Commit = trimmed
	}
	if trimmed := strings.TrimSpace(buildDate); trimmed != "" {
		info.BuildDate = trimmed
	}
	if strings.TrimSpace(info.Version) == "" {
		info.Version = "dev"
	}
	if strings.TrimSpace(info.Commit) == "" {
		info.Commit = "unknown"
	}
	if strings.TrimSpace(info.BuildDate) == "" {
		info.BuildDate = "unknown"
	}
	return info
}

func loadBuildInfo() buildInfo {
	info := buildInfo{Version: "dev"}

	goInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	if goInfo.Main.Version != "" && goInfo.Main.Version != "(devel)" {
		info.Version = goInfo.Main.Version
	}
	for _, setting := range goInfo.Settings {
		switch setting.Key {
		case "vcs.revision":
			info.Commit = setting.Value
		case "vcs.time":
			info.BuildDate = setting.Value
		}
	}
	return info
}
