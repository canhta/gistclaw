package main

import (
	"bytes"
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
