package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_Version_PrintsBuildMetadata(t *testing.T) {
	restore := setBuildMetadataForTest("v0.1.0", "abc1234", "2026-03-26T16:00:00Z")
	defer restore()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("version failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	for _, want := range []string{
		"version: v0.1.0",
		"commit: abc1234",
		"build_date: 2026-03-26T16:00:00Z",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("version output missing %q:\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got:\n%s", stderr.String())
	}
}

func TestRun_Version_FallsBackForDevBuilds(t *testing.T) {
	restore := setBuildMetadataForTest("", "", "")
	defer restore()

	versionBuildInfo = func() buildInfo {
		return buildInfo{
			Version:   "dev",
			Commit:    "fallback123",
			BuildDate: "2026-03-26T16:00:00Z",
		}
	}
	defer func() {
		versionBuildInfo = loadBuildInfo
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("version failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	for _, want := range []string{
		"version: dev",
		"commit: fallback123",
		"build_date: 2026-03-26T16:00:00Z",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("version output missing %q:\n%s", want, stdout.String())
		}
	}
}

func setBuildMetadataForTest(v, c, d string) func() {
	prevVersion := version
	prevCommit := commit
	prevBuildDate := buildDate
	prevLoader := versionBuildInfo

	version = v
	commit = c
	buildDate = d
	versionBuildInfo = loadBuildInfo

	return func() {
		version = prevVersion
		commit = prevCommit
		buildDate = prevBuildDate
		versionBuildInfo = prevLoader
	}
}
