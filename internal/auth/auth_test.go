package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/store"
)

func TestPasswordLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newAuthTestDB(t)
	now := time.Date(2026, time.March, 27, 1, 30, 0, 0, time.UTC)

	if err := VerifyPassword(ctx, db, "secret-pass"); !errors.Is(err, ErrPasswordNotSet) {
		t.Fatalf("expected ErrPasswordNotSet, got %v", err)
	}

	if err := SetPassword(ctx, db, "secret-pass", now); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	var hash string
	if err := db.RawDB().QueryRow(
		"SELECT value FROM settings WHERE key = ?",
		settingPasswordHash,
	).Scan(&hash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("expected argon2id hash, got %q", hash)
	}

	if err := VerifyPassword(ctx, db, "wrong-pass"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}
	if err := VerifyPassword(ctx, db, "secret-pass"); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
}

func TestLoginCreatesAndReusesDevice(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newAuthTestDB(t)
	now := time.Date(2026, time.March, 27, 2, 0, 0, 0, time.UTC)
	if err := SetPassword(ctx, db, "secret-pass", now); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	first, err := Login(ctx, db, LoginInput{
		Password:  "secret-pass",
		UserAgent: testDesktopUserAgent,
		IP:        "203.0.113.4:41000",
		TTL:       24 * time.Hour,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("first Login: %v", err)
	}
	if first.SessionCookieValue == "" {
		t.Fatal("expected session cookie value")
	}
	if first.DeviceCookieValue == "" {
		t.Fatal("expected device cookie value")
	}
	if first.Device.ID == "" {
		t.Fatal("expected device ID")
	}

	authn, err := AuthenticateSession(ctx, db, first.SessionCookieValue, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("AuthenticateSession: %v", err)
	}
	if authn.DeviceID != first.Device.ID {
		t.Fatalf("expected authenticated device %q, got %q", first.Device.ID, authn.DeviceID)
	}

	second, err := Login(ctx, db, LoginInput{
		Password:          "secret-pass",
		DeviceCookieValue: first.DeviceCookieValue,
		UserAgent:         testDesktopUserAgent,
		IP:                "203.0.113.4:42000",
		TTL:               24 * time.Hour,
		Now:               now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("second Login: %v", err)
	}
	if second.Device.ID != first.Device.ID {
		t.Fatalf("expected login to reuse device %q, got %q", first.Device.ID, second.Device.ID)
	}
	if second.Session.ID == first.Session.ID {
		t.Fatal("expected a new session ID for the second login")
	}

	devices, err := ListDevices(ctx, db, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].ActiveSessions != 2 {
		t.Fatalf("expected 2 active sessions, got %d", devices[0].ActiveSessions)
	}
	if devices[0].Blocked {
		t.Fatal("expected device to be unblocked")
	}
}

func TestBlockAndUnblockDevice(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newAuthTestDB(t)
	now := time.Date(2026, time.March, 27, 3, 0, 0, 0, time.UTC)
	if err := SetPassword(ctx, db, "secret-pass", now); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	issued, err := Login(ctx, db, LoginInput{
		Password:  "secret-pass",
		UserAgent: testMobileUserAgent,
		IP:        "203.0.113.8:43000",
		TTL:       24 * time.Hour,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := BlockDevice(ctx, db, issued.Device.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("BlockDevice: %v", err)
	}
	if _, err := AuthenticateSession(ctx, db, issued.SessionCookieValue, now.Add(2*time.Minute)); !errors.Is(err, ErrDeviceBlocked) {
		t.Fatalf("expected ErrDeviceBlocked after block, got %v", err)
	}
	if _, err := Login(ctx, db, LoginInput{
		Password:          "secret-pass",
		DeviceCookieValue: issued.DeviceCookieValue,
		UserAgent:         testMobileUserAgent,
		IP:                "203.0.113.8:43000",
		TTL:               24 * time.Hour,
		Now:               now.Add(3 * time.Minute),
	}); !errors.Is(err, ErrDeviceBlocked) {
		t.Fatalf("expected blocked device login to fail, got %v", err)
	}

	if err := UnblockDevice(ctx, db, issued.Device.ID, now.Add(4*time.Minute)); err != nil {
		t.Fatalf("UnblockDevice: %v", err)
	}
	if _, err := Login(ctx, db, LoginInput{
		Password:          "secret-pass",
		DeviceCookieValue: issued.DeviceCookieValue,
		UserAgent:         testMobileUserAgent,
		IP:                "203.0.113.8:43000",
		TTL:               24 * time.Hour,
		Now:               now.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("expected unblocked device login to succeed, got %v", err)
	}
}

func TestChangePasswordRevokesExistingSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newAuthTestDB(t)
	now := time.Date(2026, time.March, 27, 4, 0, 0, 0, time.UTC)
	if err := SetPassword(ctx, db, "secret-pass", now); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	issued, err := Login(ctx, db, LoginInput{
		Password:  "secret-pass",
		UserAgent: testDesktopUserAgent,
		IP:        "203.0.113.4:44000",
		TTL:       24 * time.Hour,
		Now:       now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := ChangePassword(ctx, db, "wrong-pass", "new-secret-pass", now.Add(2*time.Minute)); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword for wrong current password, got %v", err)
	}
	if err := ChangePassword(ctx, db, "secret-pass", "new-secret-pass", now.Add(3*time.Minute)); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := AuthenticateSession(ctx, db, issued.SessionCookieValue, now.Add(4*time.Minute)); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("expected ErrSessionRevoked after password change, got %v", err)
	}
	if err := VerifyPassword(ctx, db, "secret-pass"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected old password to fail, got %v", err)
	}
	if err := VerifyPassword(ctx, db, "new-secret-pass"); err != nil {
		t.Fatalf("expected new password to verify, got %v", err)
	}
}

func TestCleanupRemovesExpiredSessionsAndStaleDevices(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newAuthTestDB(t)
	now := time.Date(2026, time.March, 27, 5, 0, 0, 0, time.UTC)
	if err := EnsureSessionSecret(db); err != nil {
		t.Fatalf("EnsureSessionSecret: %v", err)
	}

	if _, err := db.RawDB().Exec(
		`INSERT INTO auth_devices
		 (id, token_hash, display_name, browser, platform, created_at, last_seen_at, last_ip, last_user_agent, blocked_at)
		 VALUES
		 ('dev-stale', 'hash-stale', 'Old Device', 'Chrome', 'macOS', ?, ?, '', '', NULL),
		 ('dev-keep', 'hash-keep', 'Keep Device', 'Safari', 'iOS', ?, ?, '', '', ?)`,
		now.Add(-staleDeviceMaxAge-time.Hour),
		now.Add(-staleDeviceMaxAge-time.Hour),
		now.Add(-staleDeviceMaxAge-time.Hour),
		now.Add(-staleDeviceMaxAge-time.Hour),
		now.Add(-time.Hour),
	); err != nil {
		t.Fatalf("insert auth devices: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO auth_sessions
		 (id, device_id, token_hash, created_at, last_seen_at, expires_at, revoked_at, revoke_reason)
		 VALUES
		 ('sess-expired', 'dev-keep', 'token-expired', ?, ?, ?, ?, 'expired cleanup')`,
		now.Add(-48*time.Hour),
		now.Add(-48*time.Hour),
		now.Add(-24*time.Hour),
		now.Add(-23*time.Hour),
	); err != nil {
		t.Fatalf("insert auth session: %v", err)
	}

	if err := Cleanup(ctx, db, now); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	var count int
	if err := db.RawDB().QueryRow(`SELECT count(*) FROM auth_devices WHERE id = 'dev-stale'`).Scan(&count); err != nil {
		t.Fatalf("count stale device: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected stale orphan device to be deleted, got count=%d", count)
	}
	if err := db.RawDB().QueryRow(`SELECT count(*) FROM auth_devices WHERE id = 'dev-keep'`).Scan(&count); err != nil {
		t.Fatalf("count keep device: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected blocked device to remain, got count=%d", count)
	}
	if err := db.RawDB().QueryRow(`SELECT count(*) FROM auth_sessions WHERE id = 'sess-expired'`).Scan(&count); err != nil {
		t.Fatalf("count expired session: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected expired revoked session to be deleted, got count=%d", count)
	}
}

func newAuthTestDB(t *testing.T) *store.DB {
	t.Helper()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

const (
	testDesktopUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
	testMobileUserAgent  = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.3 Mobile/15E148 Safari/604.1"
)
