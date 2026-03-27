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

type stubAuthZaloPersonalGroupsReader struct {
	groups []app.ZaloPersonalGroup
	err    error
	calls  int
}

func (s *stubAuthZaloPersonalGroupsReader) ListGroups(_ context.Context, _ zalopersonal.StoredCredentials) ([]app.ZaloPersonalGroup, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.groups, nil
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
	if !strings.Contains(stderr.String(), "Usage: gistclaw auth zalo-personal <login|logout|contacts|groups|send-text|send-image|send-file>") {
		t.Fatalf("expected zalo-personal usage, got:\n%s", stderr.String())
	}
}

func TestRun_AuthZaloPersonalGroupsPrintsGroupIDs(t *testing.T) {
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

	reader := &stubAuthZaloPersonalGroupsReader{
		groups: []app.ZaloPersonalGroup{
			{GroupID: "group-2", Name: "Backend", TotalMember: 8},
			{GroupID: "group-1", Name: "Alpha", TotalMember: 3},
		},
	}
	oldGroupsReaderFactory := newZaloPersonalGroupsReader
	newZaloPersonalGroupsReader = func() app.ZaloPersonalGroupsReader { return reader }
	t.Cleanup(func() { newZaloPersonalGroupsReader = oldGroupsReaderFactory })

	var stdout, stderr bytes.Buffer
	code := runWithInput(
		[]string{"auth", "--config", cfgPath, "zalo-personal", "groups"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("auth zalo-personal groups failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if reader.calls != 1 {
		t.Fatalf("expected groups reader to be called once, got %d", reader.calls)
	}
	for _, want := range []string{
		"group_id\tname\ttotal_member",
		"group-1\tAlpha\t3",
		"group-2\tBackend\t8",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected %q in stdout, got:\n%s", want, stdout.String())
		}
	}
}

func TestRun_AuthZaloPersonalSendTextUsesStoredCredentials(t *testing.T) {
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

	oldLoginWithCredentials := zaloPersonalProtocolLoginWithCredentials
	oldSendMessage := zaloPersonalProtocolSendMessage
	t.Cleanup(func() {
		zaloPersonalProtocolLoginWithCredentials = oldLoginWithCredentials
		zaloPersonalProtocolSendMessage = oldSendMessage
	})

	zaloPersonalProtocolLoginWithCredentials = func(_ context.Context, _ protocol.Credentials) (*protocol.Session, error) {
		return &protocol.Session{UID: "123456789"}, nil
	}
	var gotThreadID string
	var gotText string
	zaloPersonalProtocolSendMessage = func(_ context.Context, _ *protocol.Session, threadID string, threadType protocol.ThreadType, text string) (string, error) {
		if threadType != protocol.ThreadTypeUser {
			t.Fatalf("expected direct thread type, got %d", threadType)
		}
		gotThreadID = threadID
		gotText = text
		return "msg-1", nil
	}

	var stdout, stderr bytes.Buffer
	code := runWithInput(
		[]string{"auth", "--config", cfgPath, "zalo-personal", "send-text", "user-1", "xin chao"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("auth zalo-personal send-text failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if gotThreadID != "user-1" || gotText != "xin chao" {
		t.Fatalf("unexpected send-text args: threadID=%q text=%q", gotThreadID, gotText)
	}
}

func TestRun_AuthZaloPersonalSendImageUsesStoredCredentials(t *testing.T) {
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

	imagePath := filepath.Join(t.TempDir(), "tiny.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o600); err != nil {
		t.Fatalf("WriteFile image: %v", err)
	}

	oldLoginWithCredentials := zaloPersonalProtocolLoginWithCredentials
	oldUploadImage := zaloPersonalProtocolUploadImage
	oldSendImage := zaloPersonalProtocolSendImage
	t.Cleanup(func() {
		zaloPersonalProtocolLoginWithCredentials = oldLoginWithCredentials
		zaloPersonalProtocolUploadImage = oldUploadImage
		zaloPersonalProtocolSendImage = oldSendImage
	})

	zaloPersonalProtocolLoginWithCredentials = func(_ context.Context, _ protocol.Credentials) (*protocol.Session, error) {
		return &protocol.Session{UID: "123456789"}, nil
	}
	zaloPersonalProtocolUploadImage = func(_ context.Context, _ *protocol.Session, threadID string, threadType protocol.ThreadType, filePath string) (*protocol.ImageUploadResult, error) {
		if threadID != "user-1" || threadType != protocol.ThreadTypeUser || filePath != imagePath {
			t.Fatalf("unexpected upload image args: %q %d %q", threadID, threadType, filePath)
		}
		return &protocol.ImageUploadResult{PhotoID: "photo-1"}, nil
	}
	var gotCaption string
	zaloPersonalProtocolSendImage = func(_ context.Context, _ *protocol.Session, threadID string, threadType protocol.ThreadType, upload *protocol.ImageUploadResult, caption string) (string, error) {
		if threadID != "user-1" || threadType != protocol.ThreadTypeUser || upload == nil {
			t.Fatalf("unexpected send image args: threadID=%q threadType=%d upload=%+v", threadID, threadType, upload)
		}
		gotCaption = caption
		return "msg-image-1", nil
	}

	var stdout, stderr bytes.Buffer
	code := runWithInput(
		[]string{"auth", "--config", cfgPath, "zalo-personal", "send-image", "user-1", imagePath, "release evidence"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("auth zalo-personal send-image failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if gotCaption != "release evidence" {
		t.Fatalf("expected image caption to round-trip, got %q", gotCaption)
	}
}

func TestRun_AuthZaloPersonalSendFileUsesStoredCredentials(t *testing.T) {
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

	filePath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
		t.Fatalf("WriteFile file: %v", err)
	}

	oldLoginWithCredentials := zaloPersonalProtocolLoginWithCredentials
	oldUploadFile := zaloPersonalProtocolUploadFile
	oldSendFile := zaloPersonalProtocolSendFile
	t.Cleanup(func() {
		zaloPersonalProtocolLoginWithCredentials = oldLoginWithCredentials
		zaloPersonalProtocolUploadFile = oldUploadFile
		zaloPersonalProtocolSendFile = oldSendFile
	})

	zaloPersonalProtocolLoginWithCredentials = func(_ context.Context, _ protocol.Credentials) (*protocol.Session, error) {
		return &protocol.Session{UID: "123456789"}, nil
	}
	zaloPersonalProtocolUploadFile = func(_ context.Context, _ *protocol.Session, threadID string, threadType protocol.ThreadType, filePathArg string) (*protocol.FileUploadResult, error) {
		if threadID != "user-1" || threadType != protocol.ThreadTypeUser || filePathArg != filePath {
			t.Fatalf("unexpected upload file args: %q %d %q", threadID, threadType, filePathArg)
		}
		return &protocol.FileUploadResult{FileID: "file-1"}, nil
	}
	var gotFileID string
	zaloPersonalProtocolSendFile = func(_ context.Context, _ *protocol.Session, threadID string, threadType protocol.ThreadType, upload *protocol.FileUploadResult) (string, error) {
		if threadID != "user-1" || threadType != protocol.ThreadTypeUser || upload == nil {
			t.Fatalf("unexpected send file args: threadID=%q threadType=%d upload=%+v", threadID, threadType, upload)
		}
		gotFileID = upload.FileID
		return "msg-file-1", nil
	}

	var stdout, stderr bytes.Buffer
	code := runWithInput(
		[]string{"auth", "--config", cfgPath, "zalo-personal", "send-file", "user-1", filePath},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("auth zalo-personal send-file failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if gotFileID != "file-1" {
		t.Fatalf("expected file upload metadata to reach send, got %q", gotFileID)
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

func TestZaloPersonalProtocolFriendsReaderMapsProtocolResults(t *testing.T) {
	oldLoginWithCredentials := zaloPersonalProtocolLoginWithCredentials
	oldFetchFriends := zaloPersonalProtocolFetchFriends
	t.Cleanup(func() {
		zaloPersonalProtocolLoginWithCredentials = oldLoginWithCredentials
		zaloPersonalProtocolFetchFriends = oldFetchFriends
	})

	zaloPersonalProtocolLoginWithCredentials = func(_ context.Context, _ protocol.Credentials) (*protocol.Session, error) {
		return &protocol.Session{UID: "123456789"}, nil
	}
	zaloPersonalProtocolFetchFriends = func(_ context.Context, _ *protocol.Session) ([]protocol.FriendInfo, error) {
		return []protocol.FriendInfo{
			{UserID: "user-2", DisplayName: "Bao", ZaloName: "Bao Nguyen", Avatar: "https://example.com/b.png"},
		}, nil
	}

	got, err := (zaloPersonalProtocolFriendsReader{}).ListFriends(context.Background(), zalopersonal.StoredCredentials{
		IMEI:      "imei-123",
		Cookie:    "zpw_sek=abc123",
		UserAgent: "Mozilla/5.0",
		Language:  "vi",
	})
	if err != nil {
		t.Fatalf("ListFriends: %v", err)
	}
	if len(got) != 1 || got[0].UserID != "user-2" || got[0].DisplayName != "Bao" {
		t.Fatalf("unexpected friends: %+v", got)
	}
}

func TestZaloPersonalProtocolGroupsReaderMapsProtocolResults(t *testing.T) {
	oldLoginWithCredentials := zaloPersonalProtocolLoginWithCredentials
	oldFetchGroups := zaloPersonalProtocolFetchGroups
	t.Cleanup(func() {
		zaloPersonalProtocolLoginWithCredentials = oldLoginWithCredentials
		zaloPersonalProtocolFetchGroups = oldFetchGroups
	})

	zaloPersonalProtocolLoginWithCredentials = func(_ context.Context, _ protocol.Credentials) (*protocol.Session, error) {
		return &protocol.Session{UID: "123456789"}, nil
	}
	zaloPersonalProtocolFetchGroups = func(_ context.Context, _ *protocol.Session) ([]protocol.GroupListInfo, error) {
		return []protocol.GroupListInfo{
			{GroupID: "group-1", Name: "Alpha", Avatar: "https://example.com/a.png", TotalMember: 3},
		}, nil
	}

	got, err := (zaloPersonalProtocolGroupsReader{}).ListGroups(context.Background(), zalopersonal.StoredCredentials{
		IMEI:      "imei-123",
		Cookie:    "zpw_sek=abc123",
		UserAgent: "Mozilla/5.0",
	})
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(got) != 1 || got[0].GroupID != "group-1" || got[0].Name != "Alpha" {
		t.Fatalf("unexpected groups: %+v", got)
	}
}

func TestZaloPersonalThreadTypeUsesAllowlist(t *testing.T) {
	cfg := app.Config{
		ZaloPersonal: app.ZaloPersonalConfig{
			Groups: app.ZaloPersonalGroupsConfig{
				Allowlist: []string{"group-1"},
			},
		},
	}
	if got := zaloPersonalThreadType(cfg, "group-1"); got != protocol.ThreadTypeGroup {
		t.Fatalf("expected group thread type, got %d", got)
	}
	if got := zaloPersonalThreadType(cfg, "user-1"); got != protocol.ThreadTypeUser {
		t.Fatalf("expected user thread type, got %d", got)
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
