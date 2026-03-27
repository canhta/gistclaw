package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_InspectSystemdUnit_PrintsCanonicalUnit(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := run([]string{"inspect", "systemd-unit"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect systemd-unit failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	for _, want := range []string{
		"[Unit]",
		"[Service]",
		"User=gistclaw",
		"Group=gistclaw",
		"ExecStart=/usr/local/bin/gistclaw --config /etc/gistclaw/config.yaml serve",
		"WorkingDirectory=/var/lib/gistclaw",
		"Restart=on-failure",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("systemd unit missing %q:\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got:\n%s", stderr.String())
	}
}

func TestRun_InspectConfigPaths_PrintsInstallerOwnedDirectories(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := `
provider:
  name: openai
  api_key: sk-test
  base_url: https://example.invalid/v1
  wire_api: chat_completions
database_path: /srv/gistclaw/data/runtime.db
storage_root: /srv/gistclaw/storage
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"--config", cfgPath, "inspect", "config-paths"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect config-paths failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	for _, want := range []string{
		"state_dir: /var/lib/gistclaw",
		"database_dir: /srv/gistclaw/data",
		"storage_root: /srv/gistclaw/storage",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("config-paths output missing %q:\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got:\n%s", stderr.String())
	}
}

func TestRun_InspectConfigPaths_FailsOnInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := `
provider:
  name: openai
storage_root: /srv/gistclaw/storage
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"--config", cfgPath, "inspect", "config-paths"}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected inspect config-paths to fail:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "inspect config-paths failed") {
		t.Fatalf("expected config-paths failure in stderr, got:\n%s", stderr.String())
	}
}
