package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/app"
	"github.com/canhta/gistclaw/internal/connectors/zalopersonal"
	"github.com/canhta/gistclaw/internal/store"
)

type stubAuthZaloPersonalRunner struct {
	creds zalopersonal.StoredCredentials
	qrPNG []byte
	err   error
	calls int
}

func (s *stubAuthZaloPersonalRunner) LoginQR(_ context.Context, qrCallback func([]byte)) (zalopersonal.StoredCredentials, error) {
	s.calls++
	if len(s.qrPNG) > 0 && qrCallback != nil {
		qrCallback(s.qrPNG)
	}
	if s.err != nil {
		return zalopersonal.StoredCredentials{}, s.err
	}
	return s.creds, nil
}

func TestRun_AuthZaloPersonalLoginWritesQRFileAndStoresCredentials(t *testing.T) {
	startMockAnthropicServer(t)
	cfgPath, dbPath, stateDir := writeZaloPersonalCLIConfig(t)

	runner := &stubAuthZaloPersonalRunner{
		creds: zalopersonal.StoredCredentials{
			AccountID: "123456789",
			IMEI:      "imei-123",
			Cookie:    "zpw_sek=abc123",
			UserAgent: "Mozilla/5.0",
			Language:  "vi",
		},
		qrPNG: []byte("png-bytes"),
	}

	oldFactory := newZaloPersonalQRRunner
	newZaloPersonalQRRunner = func() app.ZaloPersonalQRLoginRunner { return runner }
	t.Cleanup(func() { newZaloPersonalQRRunner = oldFactory })

	var stdout, stderr bytes.Buffer
	code := runWithInput(
		[]string{"auth", "--config", cfgPath, "zalo-personal", "login"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("auth zalo-personal login failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if runner.calls != 1 {
		t.Fatalf("expected QR runner to be called once, got %d", runner.calls)
	}

	output := stdout.String()
	if !strings.Contains(output, "Scan QR image: ") {
		t.Fatalf("expected QR image path in stdout, got:\n%s", output)
	}
	qrPath := strings.TrimSpace(strings.TrimPrefix(output, "Scan QR image: "))
	if !strings.HasPrefix(qrPath, stateDir) {
		t.Fatalf("expected QR path under %q, got %q", stateDir, qrPath)
	}
	data, err := os.ReadFile(qrPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", qrPath, err)
	}
	if string(data) != "png-bytes" {
		t.Fatalf("expected QR file bytes %q, got %q", "png-bytes", string(data))
	}

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	got, ok, err := zalopersonal.LoadStoredCredentials(context.Background(), db)
	if err != nil {
		t.Fatalf("LoadStoredCredentials: %v", err)
	}
	if !ok {
		t.Fatal("expected stored credentials after login")
	}
	if got != runner.creds {
		t.Fatalf("expected stored creds %+v, got %+v", runner.creds, got)
	}
}

func TestRun_AuthZaloPersonalLogoutClearsCredentials(t *testing.T) {
	startMockAnthropicServer(t)
	cfgPath, dbPath, _ := writeZaloPersonalCLIConfig(t)

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	if err := zalopersonal.SaveStoredCredentials(context.Background(), db, zalopersonal.StoredCredentials{
		AccountID: "123456789",
		IMEI:      "imei-123",
		Cookie:    "zpw_sek=abc123",
		UserAgent: "Mozilla/5.0",
	}); err != nil {
		t.Fatalf("SaveStoredCredentials: %v", err)
	}
	_ = db.Close()

	var stdout, stderr bytes.Buffer
	code := runWithInput(
		[]string{"auth", "--config", cfgPath, "zalo-personal", "logout"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("auth zalo-personal logout failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "zalo personal credentials cleared") {
		t.Fatalf("expected logout confirmation, got:\n%s", stdout.String())
	}

	db, err = store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db.Close()
	_, ok, err := zalopersonal.LoadStoredCredentials(context.Background(), db)
	if err != nil {
		t.Fatalf("LoadStoredCredentials: %v", err)
	}
	if ok {
		t.Fatal("expected credentials to be cleared")
	}
}

func TestRun_AuthZaloPersonalInvalidSubcommandShowsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runWithInput(
		[]string{"auth", "zalo-personal", "wat"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if code == 0 {
		t.Fatal("expected invalid subcommand to fail")
	}
	if !strings.Contains(stderr.String(), "Usage: gistclaw auth zalo-personal <login|logout>") {
		t.Fatalf("expected zalo-personal usage, got:\n%s", stderr.String())
	}
}

func writeZaloPersonalCLIConfig(t *testing.T) (string, string, string) {
	t.Helper()

	dir := t.TempDir()
	workspaceRoot := filepath.Join(dir, "workspace")
	stateDir := filepath.Join(dir, "state")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	dbPath := filepath.Join(stateDir, "runtime.db")
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := strings.Join([]string{
		"storage_root: " + workspaceRoot,
		"state_dir: " + stateDir,
		"database_path: " + dbPath,
		"provider:",
		"  name: anthropic",
		"  api_key: sk-test",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return cfgPath, dbPath, stateDir
}
