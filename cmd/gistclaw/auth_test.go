package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	authpkg "github.com/canhta/gistclaw/internal/auth"
	"github.com/canhta/gistclaw/internal/store"
)

func TestRun_AuthSetPasswordFromStdin(t *testing.T) {
	startMockAnthropicServer(t)
	cfgPath, dbPath := writeCLIConfig(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithInput(
		[]string{"auth", "--config", cfgPath, "set-password", "--password-stdin"},
		strings.NewReader("stdin-secret-pass\n"),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("auth set-password --password-stdin failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "password updated") {
		t.Fatalf("expected success output, got:\n%s", stdout.String())
	}

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := authpkg.VerifyPassword(context.Background(), db, "stdin-secret-pass"); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
}

func TestRun_AuthSetPasswordInteractivePrompt(t *testing.T) {
	startMockAnthropicServer(t)
	cfgPath, dbPath := writeCLIConfig(t)

	oldIsTerminal := authIsTerminal
	oldReadPassword := authReadPassword
	t.Cleanup(func() {
		authIsTerminal = oldIsTerminal
		authReadPassword = oldReadPassword
	})

	authIsTerminal = func(int) bool { return true }
	var prompts int
	authReadPassword = func(int) ([]byte, error) {
		prompts++
		if prompts == 1 {
			return []byte("prompt-secret-pass"), nil
		}
		return []byte("prompt-secret-pass"), nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithInput(
		[]string{"auth", "--config", cfgPath, "set-password"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("interactive auth set-password failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if prompts != 2 {
		t.Fatalf("expected 2 password prompts, got %d", prompts)
	}
	if !strings.Contains(stderr.String(), "New password:") || !strings.Contains(stderr.String(), "Confirm password:") {
		t.Fatalf("expected prompt text in stderr, got:\n%s", stderr.String())
	}

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := authpkg.VerifyPassword(context.Background(), db, "prompt-secret-pass"); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
}

func TestRun_AuthSetPasswordRejectsNonTTYWithoutPasswordStdin(t *testing.T) {
	startMockAnthropicServer(t)
	cfgPath, _ := writeCLIConfig(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithInput(
		[]string{"auth", "--config", cfgPath, "set-password"},
		strings.NewReader("not-a-tty"),
		&stdout,
		&stderr,
	)
	if code == 0 {
		t.Fatal("expected auth set-password to reject non-tty stdin without --password-stdin")
	}
	if !strings.Contains(stderr.String(), "use --password-stdin") {
		t.Fatalf("expected non-tty guidance in stderr, got:\n%s", stderr.String())
	}
}

func TestRun_AuthSetPasswordRejectsMismatchedConfirmation(t *testing.T) {
	startMockAnthropicServer(t)
	cfgPath, _ := writeCLIConfig(t)

	oldIsTerminal := authIsTerminal
	oldReadPassword := authReadPassword
	t.Cleanup(func() {
		authIsTerminal = oldIsTerminal
		authReadPassword = oldReadPassword
	})

	authIsTerminal = func(int) bool { return true }
	var prompts int
	authReadPassword = func(int) ([]byte, error) {
		prompts++
		if prompts == 1 {
			return []byte("prompt-secret-pass"), nil
		}
		return []byte("different-pass"), nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithInput(
		[]string{"auth", "--config", cfgPath, "set-password"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if code == 0 {
		t.Fatal("expected mismatched confirmation to fail")
	}
	if !strings.Contains(stderr.String(), "passwords do not match") {
		t.Fatalf("expected mismatch message, got:\n%s", stderr.String())
	}
}
