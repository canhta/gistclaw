package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_NoArgsShowsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{}, &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit for no args")
	}
	if !strings.Contains(stderr.String(), "Usage") {
		t.Errorf("expected usage in stderr:\n%s", stderr.String())
	}
}

func TestRun_HelpFlag(t *testing.T) {
	for _, flag := range []string{"-h", "--help", "help"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{flag}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("%s: expected exit 0, got %d", flag, code)
		}
		if !strings.Contains(stdout.String(), "Usage") {
			t.Errorf("%s: expected Usage in stdout:\n%s", flag, stdout.String())
		}
		for _, want := range []string{"version", "inspect systemd-unit", "inspect config-paths"} {
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("%s: expected %q in stdout:\n%s", flag, want, stdout.String())
			}
		}
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"unknowncmd"}, &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit for unknown command")
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("expected 'unknown command' in stderr:\n%s", stderr.String())
	}
}

func TestRun_SecurityWithoutSubcommandShowsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"security"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit without security subcommand")
	}
	if !strings.Contains(stderr.String(), "Usage: gistclaw security audit") {
		t.Fatalf("expected security usage in stderr, got:\n%s", stderr.String())
	}
}

func TestRun_BackupNoDBFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"backup"}, &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit for backup with no --db")
	}
}

func TestRun_ExportNoFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"export"}, &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit for export with no flags")
	}
}

func TestParseGlobalOptions_EnvVar(t *testing.T) {
	t.Setenv("GISTCLAW_CONFIG", "/tmp/from-env.yaml")
	opts, rest, err := parseGlobalOptions([]string{"serve"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.ConfigPath != "/tmp/from-env.yaml" {
		t.Errorf("expected /tmp/from-env.yaml, got %q", opts.ConfigPath)
	}
	if len(rest) != 1 || rest[0] != "serve" {
		t.Errorf("expected rest=[serve], got %v", rest)
	}
}

func TestParseGlobalOptions_ExplicitFlag(t *testing.T) {
	t.Setenv("GISTCLAW_CONFIG", "")
	opts, rest, err := parseGlobalOptions([]string{"-c", "/tmp/explicit.yaml", "serve"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.ConfigPath != "/tmp/explicit.yaml" {
		t.Errorf("expected /tmp/explicit.yaml, got %q", opts.ConfigPath)
	}
	if len(rest) != 1 || rest[0] != "serve" {
		t.Errorf("expected rest=[serve], got %v", rest)
	}
}

func TestParseGlobalOptions_MissingValue(t *testing.T) {
	_, _, err := parseGlobalOptions([]string{"-c"})
	if err == nil {
		t.Error("expected error for -c with no value")
	}
}

func TestParseGlobalOptions_PermissionOverrides(t *testing.T) {
	opts, rest, err := parseGlobalOptions([]string{"--dangerously-skip-permissions", "--elevated-host-access", "serve"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.ApprovalMode != "auto_approve" {
		t.Fatalf("expected auto_approve, got %q", opts.ApprovalMode)
	}
	if opts.HostAccessMode != "elevated" {
		t.Fatalf("expected elevated, got %q", opts.HostAccessMode)
	}
	if len(rest) != 1 || rest[0] != "serve" {
		t.Fatalf("unexpected remaining args: %#v", rest)
	}
}
