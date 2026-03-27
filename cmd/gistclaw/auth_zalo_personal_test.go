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
	"github.com/canhta/gistclaw/internal/connectors/zalopersonal/protocol"
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

type stubAuthZaloPersonalFriendsReader struct {
	friends []app.ZaloPersonalFriend
	err     error
	calls   int
}

func (s *stubAuthZaloPersonalFriendsReader) ListFriends(_ context.Context, _ zalopersonal.StoredCredentials) ([]app.ZaloPersonalFriend, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.friends, nil
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

func TestRun_AuthZaloPersonalContactsPrintsFriendIDs(t *testing.T) {
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
		Language:  "vi",
	}); err != nil {
		t.Fatalf("SaveStoredCredentials: %v", err)
	}
	_ = db.Close()

	reader := &stubAuthZaloPersonalFriendsReader{
		friends: []app.ZaloPersonalFriend{
			{UserID: "user-2", DisplayName: "Bao"},
			{UserID: "user-1", DisplayName: "An"},
		},
	}
	oldReaderFactory := newZaloPersonalFriendsReader
	newZaloPersonalFriendsReader = func() app.ZaloPersonalFriendsReader { return reader }
	t.Cleanup(func() { newZaloPersonalFriendsReader = oldReaderFactory })

	var stdout, stderr bytes.Buffer
	code := runWithInput(
		[]string{"auth", "--config", cfgPath, "zalo-personal", "contacts"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("auth zalo-personal contacts failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if reader.calls != 1 {
		t.Fatalf("expected contacts reader to be called once, got %d", reader.calls)
	}
	for _, want := range []string{
		"user_id\tdisplay_name",
		"user-1\tAn",
		"user-2\tBao",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected %q in stdout, got:\n%s", want, stdout.String())
		}
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
	if !strings.Contains(stderr.String(), "Usage: gistclaw auth zalo-personal <login|logout|contacts>") {
		t.Fatalf("expected zalo-personal usage, got:\n%s", stderr.String())
	}
}

func TestZaloPersonalProtocolQRRunnerCopiesDisplayName(t *testing.T) {
	lang := "vi"

	oldLoginQR := zaloPersonalProtocolLoginQR
	oldLoginWithCredentials := zaloPersonalProtocolLoginWithCredentials
	t.Cleanup(func() {
		zaloPersonalProtocolLoginQR = oldLoginQR
		zaloPersonalProtocolLoginWithCredentials = oldLoginWithCredentials
	})

	zaloPersonalProtocolLoginQR = func(_ context.Context, _ func([]byte)) (*protocol.Credentials, error) {
		return &protocol.Credentials{
			IMEI:        "imei-123",
			Cookie:      "zpw_sek=abc123",
			UserAgent:   "Mozilla/5.0",
			Language:    &lang,
			DisplayName: "Canh",
		}, nil
	}
	zaloPersonalProtocolLoginWithCredentials = func(_ context.Context, _ protocol.Credentials) (*protocol.Session, error) {
		return &protocol.Session{UID: "123456789"}, nil
	}

	got, err := (zaloPersonalProtocolQRRunner{}).LoginQR(context.Background(), nil)
	if err != nil {
		t.Fatalf("LoginQR: %v", err)
	}
	if got.DisplayName != "Canh" {
		t.Fatalf("expected display name Canh, got %q", got.DisplayName)
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
