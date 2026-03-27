package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/canhta/gistclaw/internal/store"
)

const (
	settingPasswordHash  = "auth_password_hash"
	settingSessionSecret = "auth_session_secret"

	defaultSessionTTL       = 24 * time.Hour
	sessionCleanupLimit     = 128
	expiredSessionRetention = 24 * time.Hour
	staleDeviceMaxAge       = 90 * 24 * time.Hour
)

var (
	ErrPasswordNotSet   = errors.New("auth: password not set")
	ErrPasswordRequired = errors.New("auth: password is required")
	ErrInvalidPassword  = errors.New("auth: invalid password")
	ErrInvalidSession   = errors.New("auth: invalid session")
	ErrSessionExpired   = errors.New("auth: session expired")
	ErrSessionRevoked   = errors.New("auth: session revoked")
	ErrDeviceBlocked    = errors.New("auth: device blocked")
	ErrDeviceNotFound   = errors.New("auth: device not found")
)

type LoginInput struct {
	Password          string
	DeviceCookieValue string
	UserAgent         string
	IP                string
	TTL               time.Duration
	Now               time.Time
}

type IssuedSession struct {
	Device             Device
	Session            Session
	DeviceCookieValue  string
	SessionCookieValue string
}

type Device struct {
	ID            string
	DisplayName   string
	Browser       string
	Platform      string
	LastIP        string
	LastUserAgent string
	CreatedAt     time.Time
	LastSeenAt    time.Time
	Blocked       bool
	BlockedAt     *time.Time
}

type Session struct {
	ID         string
	DeviceID   string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

type SessionState struct {
	SessionID   string
	DeviceID    string
	DisplayName string
	ExpiresAt   time.Time
}

type DeviceAccess struct {
	Device
	ActiveSessions int
}

func PasswordConfigured(ctx context.Context, db *store.DB) (bool, error) {
	hash, err := loadSetting(ctx, db, settingPasswordHash)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(hash) != "", nil
}

func EnsureSessionSecret(db *store.DB) error {
	existing, err := loadSetting(context.Background(), db, settingSessionSecret)
	if err != nil {
		return err
	}
	if strings.TrimSpace(existing) != "" {
		return nil
	}
	secret, err := randomHex(32)
	if err != nil {
		return fmt.Errorf("auth: generate session secret: %w", err)
	}
	if err := upsertSetting(context.Background(), db.RawDB(), settingSessionSecret, secret); err != nil {
		return err
	}
	return nil
}

func SetPassword(ctx context.Context, db *store.DB, password string, now time.Time) error {
	now = normalizeTime(now)
	if err := EnsureSessionSecret(db); err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	tx, err := db.RawDB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("auth: begin set password: %w", err)
	}
	defer tx.Rollback()
	if err := upsertSetting(ctx, tx, settingPasswordHash, hash); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE auth_sessions
		 SET revoked_at = ?, revoke_reason = 'password changed'
		 WHERE revoked_at IS NULL`,
		now,
	); err != nil {
		return fmt.Errorf("auth: revoke sessions on password set: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("auth: commit set password: %w", err)
	}
	return nil
}

func VerifyPassword(ctx context.Context, db *store.DB, password string) error {
	hash, err := loadSetting(ctx, db, settingPasswordHash)
	if err != nil {
		return err
	}
	if strings.TrimSpace(hash) == "" {
		return ErrPasswordNotSet
	}
	ok, err := verifyPasswordHash(hash, password)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidPassword
	}
	return nil
}

func ChangePassword(ctx context.Context, db *store.DB, currentPassword, newPassword string, now time.Time) error {
	if err := VerifyPassword(ctx, db, currentPassword); err != nil {
		return err
	}
	return SetPassword(ctx, db, newPassword, now)
}

func Login(ctx context.Context, db *store.DB, input LoginInput) (IssuedSession, error) {
	if err := VerifyPassword(ctx, db, input.Password); err != nil {
		return IssuedSession{}, err
	}
	if err := EnsureSessionSecret(db); err != nil {
		return IssuedSession{}, err
	}

	now := normalizeTime(input.Now)
	if input.TTL <= 0 {
		input.TTL = defaultSessionTTL
	}
	secret, err := loadSetting(ctx, db, settingSessionSecret)
	if err != nil {
		return IssuedSession{}, err
	}

	tx, err := db.RawDB().BeginTx(ctx, nil)
	if err != nil {
		return IssuedSession{}, fmt.Errorf("auth: begin login: %w", err)
	}
	defer tx.Rollback()

	device, rawDeviceToken, err := loadOrCreateDevice(ctx, tx, secret, input.DeviceCookieValue, deviceMetadata{
		UserAgent: input.UserAgent,
		IP:        input.IP,
		Now:       now,
	})
	if err != nil {
		return IssuedSession{}, err
	}

	rawSessionToken, err := randomToken()
	if err != nil {
		return IssuedSession{}, fmt.Errorf("auth: generate session token: %w", err)
	}
	session := Session{
		ID:         newID("sess"),
		DeviceID:   device.ID,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(input.TTL),
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO auth_sessions
		 (id, device_id, token_hash, created_at, last_seen_at, expires_at, revoked_at, revoke_reason)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, '')`,
		session.ID,
		session.DeviceID,
		hashToken(rawSessionToken),
		session.CreatedAt,
		session.LastSeenAt,
		session.ExpiresAt,
	); err != nil {
		return IssuedSession{}, fmt.Errorf("auth: insert session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return IssuedSession{}, fmt.Errorf("auth: commit login: %w", err)
	}
	if err := Cleanup(ctx, db, now); err != nil {
		return IssuedSession{}, err
	}

	return IssuedSession{
		Device:             device,
		Session:            session,
		DeviceCookieValue:  signToken(secret, rawDeviceToken),
		SessionCookieValue: signToken(secret, rawSessionToken),
	}, nil
}

func AuthenticateSession(ctx context.Context, db *store.DB, sessionCookieValue string, now time.Time) (SessionState, error) {
	now = normalizeTime(now)
	secret, err := loadSetting(ctx, db, settingSessionSecret)
	if err != nil {
		return SessionState{}, err
	}
	rawToken, err := verifySignedToken(secret, sessionCookieValue)
	if err != nil {
		return SessionState{}, ErrInvalidSession
	}

	var (
		sessionID   string
		deviceID    string
		displayName string
		expiresAt   time.Time
		revokedAt   sql.NullTime
		blockedAt   sql.NullTime
	)
	if err := db.RawDB().QueryRowContext(ctx,
		`SELECT s.id, d.id, d.display_name, s.expires_at, s.revoked_at, d.blocked_at
		 FROM auth_sessions s
		 JOIN auth_devices d ON d.id = s.device_id
		 WHERE s.token_hash = ?`,
		hashToken(rawToken),
	).Scan(&sessionID, &deviceID, &displayName, &expiresAt, &revokedAt, &blockedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionState{}, ErrInvalidSession
		}
		return SessionState{}, fmt.Errorf("auth: load session: %w", err)
	}
	if blockedAt.Valid {
		return SessionState{}, ErrDeviceBlocked
	}
	if revokedAt.Valid {
		return SessionState{}, ErrSessionRevoked
	}
	if !expiresAt.After(now) {
		return SessionState{}, ErrSessionExpired
	}
	if _, err := db.RawDB().ExecContext(ctx,
		`UPDATE auth_sessions SET last_seen_at = ? WHERE id = ?`,
		now,
		sessionID,
	); err != nil {
		return SessionState{}, fmt.Errorf("auth: touch session: %w", err)
	}
	if _, err := db.RawDB().ExecContext(ctx,
		`UPDATE auth_devices SET last_seen_at = ? WHERE id = ?`,
		now,
		deviceID,
	); err != nil {
		return SessionState{}, fmt.Errorf("auth: touch device: %w", err)
	}
	return SessionState{
		SessionID:   sessionID,
		DeviceID:    deviceID,
		DisplayName: displayName,
		ExpiresAt:   expiresAt,
	}, nil
}

func LogoutSession(ctx context.Context, db *store.DB, sessionCookieValue string, now time.Time) error {
	now = normalizeTime(now)
	secret, err := loadSetting(ctx, db, settingSessionSecret)
	if err != nil {
		return err
	}
	rawToken, err := verifySignedToken(secret, sessionCookieValue)
	if err != nil {
		return nil
	}
	if _, err := db.RawDB().ExecContext(ctx,
		`UPDATE auth_sessions
		 SET revoked_at = COALESCE(revoked_at, ?), revoke_reason = CASE WHEN revoke_reason = '' THEN 'logout' ELSE revoke_reason END
		 WHERE token_hash = ?`,
		now,
		hashToken(rawToken),
	); err != nil {
		return fmt.Errorf("auth: logout session: %w", err)
	}
	return Cleanup(ctx, db, now)
}

func BlockDevice(ctx context.Context, db *store.DB, deviceID string, now time.Time) error {
	return mutateDevice(ctx, db, deviceID, now, true)
}

func UnblockDevice(ctx context.Context, db *store.DB, deviceID string, now time.Time) error {
	return mutateDevice(ctx, db, deviceID, now, false)
}

func RevokeDeviceSessions(ctx context.Context, db *store.DB, deviceID string, now time.Time) error {
	now = normalizeTime(now)
	res, err := db.RawDB().ExecContext(ctx,
		`UPDATE auth_sessions
		 SET revoked_at = COALESCE(revoked_at, ?), revoke_reason = CASE WHEN revoke_reason = '' THEN 'device revoked' ELSE revoke_reason END
		 WHERE device_id = ? AND revoked_at IS NULL`,
		now,
		deviceID,
	)
	if err != nil {
		return fmt.Errorf("auth: revoke device sessions: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		var count int
		if err := db.RawDB().QueryRowContext(ctx, `SELECT count(*) FROM auth_devices WHERE id = ?`, deviceID).Scan(&count); err != nil {
			return fmt.Errorf("auth: verify device: %w", err)
		}
		if count == 0 {
			return ErrDeviceNotFound
		}
	}
	return Cleanup(ctx, db, now)
}

func ListDevices(ctx context.Context, db *store.DB, now time.Time) ([]DeviceAccess, error) {
	rows, err := db.RawDB().QueryContext(ctx,
		`SELECT d.id, d.display_name, d.browser, d.platform, d.created_at, d.last_seen_at,
		        d.last_ip, d.last_user_agent, d.blocked_at,
		        COALESCE(SUM(CASE WHEN s.revoked_at IS NULL AND s.expires_at > ? THEN 1 ELSE 0 END), 0) AS active_sessions
		 FROM auth_devices d
		 LEFT JOIN auth_sessions s ON s.device_id = d.id
		 GROUP BY d.id, d.display_name, d.browser, d.platform, d.created_at, d.last_seen_at, d.last_ip, d.last_user_agent, d.blocked_at
		 ORDER BY CASE WHEN d.blocked_at IS NULL THEN 0 ELSE 1 END, d.last_seen_at DESC, d.created_at DESC`,
		normalizeTime(now),
	)
	if err != nil {
		return nil, fmt.Errorf("auth: list devices: %w", err)
	}
	defer rows.Close()

	var devices []DeviceAccess
	for rows.Next() {
		var (
			device    DeviceAccess
			blockedAt sql.NullTime
		)
		if err := rows.Scan(
			&device.ID,
			&device.DisplayName,
			&device.Browser,
			&device.Platform,
			&device.CreatedAt,
			&device.LastSeenAt,
			&device.LastIP,
			&device.LastUserAgent,
			&blockedAt,
			&device.ActiveSessions,
		); err != nil {
			return nil, fmt.Errorf("auth: scan device: %w", err)
		}
		if blockedAt.Valid {
			ts := blockedAt.Time
			device.Blocked = true
			device.BlockedAt = &ts
		}
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: list devices rows: %w", err)
	}
	return devices, nil
}

func Cleanup(ctx context.Context, db *store.DB, now time.Time) error {
	now = normalizeTime(now)
	sessionCutoff := now.Add(-expiredSessionRetention)
	if _, err := db.RawDB().ExecContext(ctx,
		`DELETE FROM auth_sessions
		 WHERE id IN (
		 	SELECT id FROM auth_sessions
		 	WHERE (revoked_at IS NOT NULL AND revoked_at <= ?)
		 	   OR expires_at <= ?
		 	ORDER BY COALESCE(revoked_at, expires_at)
		 	LIMIT ?
		 )`,
		sessionCutoff,
		sessionCutoff,
		sessionCleanupLimit,
	); err != nil {
		return fmt.Errorf("auth: cleanup sessions: %w", err)
	}

	deviceCutoff := now.Add(-staleDeviceMaxAge)
	if _, err := db.RawDB().ExecContext(ctx,
		`DELETE FROM auth_devices
		 WHERE id IN (
		 	SELECT d.id
		 	FROM auth_devices d
		 	LEFT JOIN auth_sessions s ON s.device_id = d.id
		 	WHERE d.blocked_at IS NULL
		 	  AND d.last_seen_at < ?
		 	GROUP BY d.id
		 	HAVING COUNT(s.id) = 0
		 	ORDER BY d.last_seen_at
		 	LIMIT ?
		 )`,
		deviceCutoff,
		sessionCleanupLimit,
	); err != nil {
		return fmt.Errorf("auth: cleanup devices: %w", err)
	}

	return nil
}

type deviceMetadata struct {
	UserAgent string
	IP        string
	Now       time.Time
}

func loadOrCreateDevice(ctx context.Context, tx *sql.Tx, secret, signedCookie string, meta deviceMetadata) (Device, string, error) {
	rawToken, err := verifySignedToken(secret, signedCookie)
	if err == nil && rawToken != "" {
		device, err := findDeviceByToken(ctx, tx, rawToken)
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return Device{}, "", fmt.Errorf("auth: load device: %w", err)
		default:
			if device.Blocked {
				return Device{}, "", ErrDeviceBlocked
			}
			device = enrichDevice(device, meta)
			if _, err := tx.ExecContext(ctx,
				`UPDATE auth_devices
				 SET display_name = ?, browser = ?, platform = ?, last_seen_at = ?, last_ip = ?, last_user_agent = ?
				 WHERE id = ?`,
				device.DisplayName,
				device.Browser,
				device.Platform,
				device.LastSeenAt,
				device.LastIP,
				device.LastUserAgent,
				device.ID,
			); err != nil {
				return Device{}, "", fmt.Errorf("auth: update device: %w", err)
			}
			return device, rawToken, nil
		}
	}

	rawToken, err = randomToken()
	if err != nil {
		return Device{}, "", fmt.Errorf("auth: generate device token: %w", err)
	}
	device := enrichDevice(Device{
		ID:        newID("dev"),
		CreatedAt: meta.Now,
	}, meta)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO auth_devices
		 (id, token_hash, display_name, browser, platform, created_at, last_seen_at, last_ip, last_user_agent, blocked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		device.ID,
		hashToken(rawToken),
		device.DisplayName,
		device.Browser,
		device.Platform,
		device.CreatedAt,
		device.LastSeenAt,
		device.LastIP,
		device.LastUserAgent,
	); err != nil {
		return Device{}, "", fmt.Errorf("auth: insert device: %w", err)
	}
	return device, rawToken, nil
}

func findDeviceByToken(ctx context.Context, tx *sql.Tx, rawToken string) (Device, error) {
	var (
		device    Device
		blockedAt sql.NullTime
	)
	err := tx.QueryRowContext(ctx,
		`SELECT id, display_name, browser, platform, created_at, last_seen_at, last_ip, last_user_agent, blocked_at
		 FROM auth_devices
		 WHERE token_hash = ?`,
		hashToken(rawToken),
	).Scan(
		&device.ID,
		&device.DisplayName,
		&device.Browser,
		&device.Platform,
		&device.CreatedAt,
		&device.LastSeenAt,
		&device.LastIP,
		&device.LastUserAgent,
		&blockedAt,
	)
	if err != nil {
		return Device{}, err
	}
	if blockedAt.Valid {
		ts := blockedAt.Time
		device.Blocked = true
		device.BlockedAt = &ts
	}
	return device, nil
}

func mutateDevice(ctx context.Context, db *store.DB, deviceID string, now time.Time, blocked bool) error {
	now = normalizeTime(now)
	tx, err := db.RawDB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("auth: begin mutate device: %w", err)
	}
	defer tx.Rollback()

	var res sql.Result
	if blocked {
		res, err = tx.ExecContext(ctx,
			`UPDATE auth_devices SET blocked_at = ? WHERE id = ? AND blocked_at IS NULL`,
			now,
			deviceID,
		)
	} else {
		res, err = tx.ExecContext(ctx,
			`UPDATE auth_devices SET blocked_at = NULL WHERE id = ?`,
			deviceID,
		)
	}
	if err != nil {
		return fmt.Errorf("auth: update device block state: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM auth_devices WHERE id = ?`, deviceID).Scan(&count); err != nil {
			return fmt.Errorf("auth: verify device exists: %w", err)
		}
		if count == 0 {
			return ErrDeviceNotFound
		}
	}
	if blocked {
		if _, err := tx.ExecContext(ctx,
			`UPDATE auth_sessions
			 SET revoked_at = COALESCE(revoked_at, ?), revoke_reason = CASE WHEN revoke_reason = '' THEN 'device blocked' ELSE revoke_reason END
			 WHERE device_id = ? AND revoked_at IS NULL`,
			now,
			deviceID,
		); err != nil {
			return fmt.Errorf("auth: revoke blocked device sessions: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("auth: commit mutate device: %w", err)
	}
	return Cleanup(ctx, db, now)
}

func enrichDevice(device Device, meta deviceMetadata) Device {
	browser := detectBrowser(meta.UserAgent)
	platform := detectPlatform(meta.UserAgent)
	device.DisplayName = displayName(browser, platform)
	device.Browser = browser
	device.Platform = platform
	device.LastSeenAt = meta.Now
	device.LastIP = hostOnly(meta.IP)
	device.LastUserAgent = strings.TrimSpace(meta.UserAgent)
	if device.CreatedAt.IsZero() {
		device.CreatedAt = meta.Now
	}
	return device
}

func displayName(browser, platform string) string {
	switch {
	case browser != "" && platform != "":
		return browser + " on " + platform
	case browser != "":
		return browser
	case platform != "":
		return "Browser on " + platform
	default:
		return "Browser"
	}
}

func detectBrowser(userAgent string) string {
	switch {
	case strings.Contains(userAgent, "Edg/"):
		return "Edge"
	case strings.Contains(userAgent, "Chrome/") && !strings.Contains(userAgent, "Edg/"):
		return "Chrome"
	case strings.Contains(userAgent, "Firefox/"):
		return "Firefox"
	case strings.Contains(userAgent, "Safari/") && strings.Contains(userAgent, "Version/") && !strings.Contains(userAgent, "Chrome/"):
		return "Safari"
	default:
		return "Browser"
	}
}

func detectPlatform(userAgent string) string {
	switch {
	case strings.Contains(userAgent, "iPhone") || strings.Contains(userAgent, "iPad"):
		return "iOS"
	case strings.Contains(userAgent, "Android"):
		return "Android"
	case strings.Contains(userAgent, "Macintosh"):
		return "macOS"
	case strings.Contains(userAgent, "Windows"):
		return "Windows"
	case strings.Contains(userAgent, "Linux"):
		return "Linux"
	default:
		return "Unknown platform"
	}
}

func hostOnly(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(raw)
	if err == nil {
		return host
	}
	return raw
}

func hashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", ErrPasswordRequired
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate password salt: %w", err)
	}
	memory := uint32(19 * 1024)
	iterations := uint32(2)
	parallelism := uint8(1)
	keyLength := uint32(32)
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		memory,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifyPasswordHash(encoded, password string) (bool, error) {
	if strings.TrimSpace(password) == "" {
		return false, ErrInvalidPassword
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("auth: unsupported password hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("auth: parse password hash version: %w", err)
	}
	if version != argon2.Version {
		return false, fmt.Errorf("auth: unsupported argon2 version %d", version)
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, fmt.Errorf("auth: parse password hash params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("auth: decode password salt: %w", err)
	}
	wantHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("auth: decode password hash: %w", err)
	}
	gotHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(wantHash)))
	return subtle.ConstantTimeCompare(gotHash, wantHash) == 1, nil
}

func loadSetting(ctx context.Context, db *store.DB, key string) (string, error) {
	var value string
	err := db.RawDB().QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("auth: load setting %q: %w", key, err)
	}
	return value, nil
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func upsertSetting(ctx context.Context, exec execer, key, value string) error {
	if _, err := exec.ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key,
		value,
	); err != nil {
		return fmt.Errorf("auth: upsert setting %q: %w", key, err)
	}
	return nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func signToken(secret, raw string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(raw))
	return raw + "." + hex.EncodeToString(mac.Sum(nil))
}

func verifySignedToken(secret, signed string) (string, error) {
	if strings.TrimSpace(signed) == "" {
		return "", ErrInvalidSession
	}
	raw, sig, ok := strings.Cut(signed, ".")
	if !ok || raw == "" || sig == "" {
		return "", ErrInvalidSession
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(raw))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return "", ErrInvalidSession
	}
	return raw, nil
}

func newID(prefix string) string {
	token, err := randomHex(12)
	if err != nil {
		panic("auth: random ID generation failed: " + err.Error())
	}
	return prefix + "_" + token
}

func normalizeTime(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}
