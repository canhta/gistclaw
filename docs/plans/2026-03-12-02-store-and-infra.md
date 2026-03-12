# GistClaw Plan 2: Store & Infra

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the SQLite store and the three cross-cutting infrastructure packages (CostGuard, SOULLoader, Heartbeat) that all agent services depend on.

**Architecture:** `internal/store` is a pure data layer — schema, WAL mode, startup purge, typed query helpers. `internal/infra` bundles three small concerns into one package: `cost.go` (atomic cost tracking with daily reset), `soul.go` (mtime-cached file loader), `heartbeat.go` (Tier 1 Telegram liveness + Tier 2 agent health checks). No goroutines live in `infra` itself — callers own the loops.

**Tech Stack:** Go 1.25, `modernc.org/sqlite` (pure-Go, no CGo), `github.com/rs/zerolog`

**Design reference:** `docs/plans/design-v3.md` §6, §9.9, §9.10, §9.11, §9.12, §14

**Depends on:** Plan 1 (config, channel.Channel)

---

## Execution order

```
Task 1  internal/store  (schema + client + startup purge)
Task 2  internal/infra/cost.go
Task 3  internal/infra/soul.go
Task 4  internal/infra/heartbeat.go
```

Tasks 2–4 are independent of each other and can be written in any order after Task 1.

---

### Task 1: `internal/store` — SQLite client + schema

**Files:**
- Create: `internal/store/schema.sql`
- Create: `internal/store/sqlite.go`
- Create: `internal/store/sqlite_test.go`

**Step 1: Add dependency**

```bash
go get modernc.org/sqlite
go mod tidy
```

**Step 2: Write the failing test**

```go
// internal/store/sqlite_test.go
package store_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenCreatesSchema(t *testing.T) {
	s := newTestStore(t)
	// If schema was applied, we can query each table without error.
	tables := []string{"sessions", "hitl_pending", "cost_daily", "channel_state", "provider_credentials", "jobs"}
	for _, table := range tables {
		if err := s.Ping(table); err != nil {
			t.Errorf("table %q not accessible: %v", table, err)
		}
	}
}

func TestPurgeSessionsOlderThan(t *testing.T) {
	s := newTestStore(t)

	// Insert two sessions: one old, one recent.
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := s.InsertSession("old-id", "opencode", "active", "prompt", oldTime); err != nil {
		t.Fatalf("InsertSession old: %v", err)
	}
	if err := s.InsertSession("new-id", "opencode", "active", "prompt", time.Now()); err != nil {
		t.Fatalf("InsertSession new: %v", err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	if err := s.PurgeSessions(cutoff); err != nil {
		t.Fatalf("PurgeSessions: %v", err)
	}

	remaining, err := s.CountSessions()
	if err != nil {
		t.Fatalf("CountSessions: %v", err)
	}
	if remaining != 1 {
		t.Errorf("after purge: got %d sessions, want 1", remaining)
	}
}

func TestChannelStateUpdateID(t *testing.T) {
	s := newTestStore(t)

	// Initially no record — GetLastUpdateID returns 0.
	id, err := s.GetLastUpdateID("telegram:123")
	if err != nil {
		t.Fatalf("GetLastUpdateID: %v", err)
	}
	if id != 0 {
		t.Errorf("initial update ID: got %d, want 0", id)
	}

	if err := s.SetLastUpdateID("telegram:123", 42); err != nil {
		t.Fatalf("SetLastUpdateID: %v", err)
	}

	id, err = s.GetLastUpdateID("telegram:123")
	if err != nil {
		t.Fatalf("GetLastUpdateID after set: %v", err)
	}
	if id != 42 {
		t.Errorf("after set: got %d, want 42", id)
	}
}

func TestProviderCredentials(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetProviderCredentials("codex")
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing credential, got %v", err)
	}

	if err := s.SetProviderCredentials("codex", `{"access_token":"tok"}`); err != nil {
		t.Fatalf("SetProviderCredentials: %v", err)
	}

	data, err := s.GetProviderCredentials("codex")
	if err != nil {
		t.Fatalf("GetProviderCredentials after set: %v", err)
	}
	if data != `{"access_token":"tok"}` {
		t.Errorf("unexpected credential data: %q", data)
	}
}

func TestCostDailyUpsert(t *testing.T) {
	s := newTestStore(t)

	date := "2026-03-12"
	if err := s.UpsertCostDaily(date, 1.5); err != nil {
		t.Fatalf("UpsertCostDaily: %v", err)
	}
	if err := s.UpsertCostDaily(date, 0.5); err != nil {
		t.Fatalf("UpsertCostDaily second call: %v", err)
	}
	total, err := s.GetCostDaily(date)
	if err != nil {
		t.Fatalf("GetCostDaily: %v", err)
	}
	// Each call sets the total (not adds), so last write wins.
	if total != 0.5 {
		t.Errorf("cost daily: got %v, want 0.5", total)
	}
}

func TestHITLPending(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertHITLPending("perm_001", "opencode", "write_file"); err != nil {
		t.Fatalf("InsertHITLPending: %v", err)
	}

	pending, err := s.ListPendingHITL()
	if err != nil {
		t.Fatalf("ListPendingHITL: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "perm_001" {
		t.Errorf("ListPendingHITL: got %v", pending)
	}

	if err := s.ResolveHITL("perm_001", "auto_rejected"); err != nil {
		t.Fatalf("ResolveHITL: %v", err)
	}

	pending, err = s.ListPendingHITL()
	if err != nil {
		t.Fatalf("ListPendingHITL after resolve: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after resolve, got %d", len(pending))
	}
}

func TestPurgeOldFiles(_ *testing.T) {
	// Ensure test binary does not leave temp files.
	os.RemoveAll("./test.db")
}
```

**Step 3: Run test to verify it fails**

```bash
go test ./internal/store/...
```

Expected: `FAIL` — package does not exist.

**Step 4: Create `schema.sql`**

```sql
-- internal/store/schema.sql
-- WAL mode is set programmatically on Open(); not in this file.

CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    agent       TEXT NOT NULL,
    status      TEXT NOT NULL,
    prompt      TEXT,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at DATETIME
);

CREATE TABLE IF NOT EXISTS hitl_pending (
    id          TEXT PRIMARY KEY,
    agent       TEXT NOT NULL,
    tool_name   TEXT,
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    resolved_at DATETIME
);

CREATE TABLE IF NOT EXISTS cost_daily (
    date        TEXT PRIMARY KEY,
    total_usd   REAL NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS channel_state (
    channel_id      TEXT PRIMARY KEY,
    last_update_id  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS provider_credentials (
    provider    TEXT PRIMARY KEY,
    data        TEXT NOT NULL,
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS jobs (
    id               TEXT PRIMARY KEY,
    kind             TEXT NOT NULL,
    target           TEXT NOT NULL,
    prompt           TEXT NOT NULL,
    schedule         TEXT NOT NULL,
    next_run_at      DATETIME NOT NULL,
    last_run_at      DATETIME,
    enabled          INTEGER NOT NULL DEFAULT 1,
    delete_after_run INTEGER NOT NULL DEFAULT 0,
    created_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);
```

**Step 5: Write `sqlite.go`**

```go
// internal/store/sqlite.go
package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("store: not found")

// HITLRecord is a row from hitl_pending.
type HITLRecord struct {
	ID       string
	Agent    string
	ToolName string
	Status   string
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, enables WAL mode,
// and applies the schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("store: open %q: %w", path, err)
	}
	// Single writer; avoid "database is locked" under concurrent reads.
	db.SetMaxOpenConns(1)

	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return nil, fmt.Errorf("store: read schema: %w", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		return nil, fmt.Errorf("store: apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

// Ping verifies a table exists by selecting from it.
func (s *Store) Ping(table string) error {
	// table name is only ever passed from internal code, not user input — safe.
	_, err := s.db.Exec("SELECT 1 FROM " + table + " LIMIT 0")
	return err
}

// PurgeStartup runs all startup purge operations:
//   - sessions older than sessionTTL
//   - hitl_pending records older than hitlTTL
//   - cost_daily rows older than costTTL
func (s *Store) PurgeStartup(sessionTTL, hitlTTL, costTTL time.Duration) error {
	if err := s.PurgeSessions(time.Now().Add(-sessionTTL)); err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`DELETE FROM hitl_pending WHERE created_at < ?`,
		time.Now().Add(-hitlTTL).UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("store: purge hitl_pending: %w", err)
	}
	if _, err := s.db.Exec(
		`DELETE FROM cost_daily WHERE date < ?`,
		time.Now().Add(-costTTL).UTC().Format("2006-01-02"),
	); err != nil {
		return fmt.Errorf("store: purge cost_daily: %w", err)
	}
	return nil
}

// PurgeSessions deletes sessions with created_at before cutoff.
func (s *Store) PurgeSessions(cutoff time.Time) error {
	_, err := s.db.Exec(
		`DELETE FROM sessions WHERE created_at < ?`,
		cutoff.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("store: purge sessions: %w", err)
	}
	return nil
}

// InsertSession inserts a new session record.
func (s *Store) InsertSession(id, agent, status, prompt string, createdAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, agent, status, prompt, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, agent, status, prompt, createdAt.UTC().Format(time.RFC3339),
	)
	return err
}

// CountSessions returns the total number of session rows.
func (s *Store) CountSessions() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n)
	return n, err
}

// GetLastUpdateID returns the last seen update_id for a channel, or 0 if none.
func (s *Store) GetLastUpdateID(channelID string) (int64, error) {
	var id int64
	err := s.db.QueryRow(
		`SELECT last_update_id FROM channel_state WHERE channel_id = ?`, channelID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// SetLastUpdateID upserts the last_update_id for a channel.
func (s *Store) SetLastUpdateID(channelID string, updateID int64) error {
	_, err := s.db.Exec(
		`INSERT INTO channel_state (channel_id, last_update_id) VALUES (?, ?)
		 ON CONFLICT(channel_id) DO UPDATE SET last_update_id = excluded.last_update_id`,
		channelID, updateID,
	)
	return err
}

// GetProviderCredentials returns the stored credential JSON for provider.
// Returns ErrNotFound if no credential is stored.
func (s *Store) GetProviderCredentials(provider string) (string, error) {
	var data string
	err := s.db.QueryRow(
		`SELECT data FROM provider_credentials WHERE provider = ?`, provider,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return data, err
}

// SetProviderCredentials upserts the credential JSON for provider.
func (s *Store) SetProviderCredentials(provider, data string) error {
	_, err := s.db.Exec(
		`INSERT INTO provider_credentials (provider, data, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(provider) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
		provider, data,
	)
	return err
}

// UpsertCostDaily sets (not adds) the total_usd for date.
func (s *Store) UpsertCostDaily(date string, totalUSD float64) error {
	_, err := s.db.Exec(
		`INSERT INTO cost_daily (date, total_usd) VALUES (?, ?)
		 ON CONFLICT(date) DO UPDATE SET total_usd = excluded.total_usd`,
		date, totalUSD,
	)
	return err
}

// GetCostDaily returns the total_usd for date, or 0 if no row exists.
func (s *Store) GetCostDaily(date string) (float64, error) {
	var total float64
	err := s.db.QueryRow(`SELECT total_usd FROM cost_daily WHERE date = ?`, date).Scan(&total)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return total, err
}

// InsertHITLPending inserts a new pending HITL record with status "pending".
func (s *Store) InsertHITLPending(id, agent, toolName string) error {
	_, err := s.db.Exec(
		`INSERT INTO hitl_pending (id, agent, tool_name, status) VALUES (?, ?, ?, 'pending')`,
		id, agent, toolName,
	)
	return err
}

// ListPendingHITL returns all hitl_pending rows with status "pending".
func (s *Store) ListPendingHITL() ([]HITLRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, agent, COALESCE(tool_name,'') FROM hitl_pending WHERE status = 'pending'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HITLRecord
	for rows.Next() {
		var r HITLRecord
		r.Status = "pending"
		if err := rows.Scan(&r.ID, &r.Agent, &r.ToolName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ResolveHITL updates the status of a hitl_pending record.
func (s *Store) ResolveHITL(id, status string) error {
	_, err := s.db.Exec(
		`UPDATE hitl_pending SET status = ?, resolved_at = datetime('now') WHERE id = ?`,
		status, id,
	)
	return err
}
```

**Step 6: Run test to verify it passes**

```bash
go test ./internal/store/...
```

Expected: `PASS`

**Step 7: Commit**

```bash
git add internal/store/ go.mod go.sum
git commit -m "feat: add SQLite store with schema, WAL mode, startup purge, and typed helpers"
```

---

### Task 2: `internal/infra/cost.go`

**Files:**
- Create: `internal/infra/cost.go`
- Create: `internal/infra/cost_test.go`

**Step 1: Write the failing test**

```go
// internal/infra/cost_test.go
package infra_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/canhta/gistclaw/internal/infra"
	"github.com/canhta/gistclaw/internal/store"
)

func newTestCostGuard(t *testing.T, limitUSD float64) *infra.CostGuard {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return infra.NewCostGuard(s, limitUSD, nil) // nil notifier for unit tests
}

func TestCostGuardTrackBelowThreshold(t *testing.T) {
	g := newTestCostGuard(t, 10.0)
	ctx := context.Background()
	// Track 50% of limit — no notification expected.
	if err := g.Track(ctx, 5.0); err != nil {
		t.Fatalf("Track: %v", err)
	}
}

func TestCostGuardCurrentTotal(t *testing.T) {
	g := newTestCostGuard(t, 10.0)
	ctx := context.Background()
	_ = g.Track(ctx, 3.0)
	_ = g.Track(ctx, 2.0)
	if got := g.CurrentUSD(); got != 5.0 {
		t.Errorf("CurrentUSD: got %v, want 5.0", got)
	}
}

func TestCostGuardConcurrentTrack(t *testing.T) {
	g := newTestCostGuard(t, 100.0)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = g.Track(ctx, 1.0)
		}()
	}
	wg.Wait()
	if got := g.CurrentUSD(); got != 10.0 {
		t.Errorf("concurrent CurrentUSD: got %v, want 10.0", got)
	}
}

func TestCostGuardZeroIsNoOp(t *testing.T) {
	g := newTestCostGuard(t, 10.0)
	ctx := context.Background()
	// Providers with opaque billing return 0; Track(0) must not panic or error.
	if err := g.Track(ctx, 0); err != nil {
		t.Fatalf("Track(0): %v", err)
	}
	if got := g.CurrentUSD(); got != 0 {
		t.Errorf("after Track(0): CurrentUSD = %v, want 0", got)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/infra/...
```

Expected: `FAIL`

**Step 3: Write implementation**

```go
// internal/infra/cost.go
package infra

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/store"
)

// Notifier is a minimal interface for sending text messages.
// Implemented by channel.Channel (or a thin adapter); kept as a local interface
// to avoid an import cycle with internal/channel.
type Notifier interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

// CostGuard tracks daily LLM spend and sends soft-stop notifications at 80% and 100%.
// It is safe for concurrent use.
//
// Concurrency design:
//   - cents (atomic.Int64) is incremented lock-free; CurrentUSD() reads it without a lock.
//   - mu guards only the mutable state that cannot be atomic: today string, notified flags,
//     and the SQLite write (which must use the post-increment total).
type CostGuard struct {
	cents      atomic.Int64 // spend in integer micro-dollars (1e6 per USD); lock-free
	mu         sync.Mutex   // guards today, notified80, notified100, and SQLite write
	limitUSD   float64
	store      *store.Store
	notifier   Notifier // may be nil in tests
	operatorID int64    // chat ID to notify; 0 = no notification
	today      string   // current date string "YYYY-MM-DD"; reset daily
	notified80  bool
	notified100 bool
}

// NewCostGuard creates a CostGuard. notifier may be nil (no Telegram notifications sent).
// operatorChatID is the chat ID for supervisor notifications.
func NewCostGuard(s *store.Store, limitUSD float64, notifier Notifier) *CostGuard {
	return &CostGuard{
		store:    s,
		limitUSD: limitUSD,
		notifier: notifier,
		today:    todayUTC(),
	}
}

// WithOperator sets the operator chat ID for notifications. Returns g for chaining.
func (g *CostGuard) WithOperator(chatID int64) *CostGuard {
	g.operatorID = chatID
	return g
}

// Track adds usd to the daily spend and triggers notifications if thresholds are crossed.
// A zero usd value is a valid no-op (providers with opaque billing).
func (g *CostGuard) Track(ctx context.Context, usd float64) error {
	if usd == 0 {
		return nil
	}

	// Increment the atomic counter lock-free first.
	microDollars := int64(math.Round(usd * 1e6))
	newTotal := g.cents.Add(microDollars)
	totalUSD := float64(newTotal) / 1e6

	// Lock only for: daily reset check, notification flag check, and SQLite write.
	g.mu.Lock()
	defer g.mu.Unlock()

	// Daily reset: if the date has changed, reset counter and notification flags.
	if today := todayUTC(); today != g.today {
		g.today = today
		g.cents.Store(0)
		newTotal = 0
		totalUSD = 0
		g.notified80 = false
		g.notified100 = false
	}

	// Persist to SQLite (uses totalUSD computed after any reset).
	if err := g.store.UpsertCostDaily(g.today, totalUSD); err != nil {
		log.Error().Err(err).Msg("infra/cost: failed to persist daily cost")
	}

	// Threshold notifications (send once per day per threshold).
	if g.notifier != nil && g.operatorID != 0 {
		pct := totalUSD / g.limitUSD * 100
		if pct >= 100 && !g.notified100 {
			g.notified100 = true
			msg := fmt.Sprintf("⚠️ Daily limit reached ($%.2f / $%.2f). Current session will finish cleanly.", totalUSD, g.limitUSD)
			go g.notifier.SendMessage(ctx, g.operatorID, msg) //nolint:errcheck
		} else if pct >= 80 && !g.notified80 {
			g.notified80 = true
			msg := fmt.Sprintf("⚠️ 80%% of daily cost used ($%.2f / $%.2f).", totalUSD, g.limitUSD)
			go g.notifier.SendMessage(ctx, g.operatorID, msg) //nolint:errcheck
		}
	}

	return nil
}

// CurrentUSD returns the current daily spend in USD.
// Reads the atomic counter lock-free — safe for concurrent calls.
func (g *CostGuard) CurrentUSD() float64 {
	return float64(g.cents.Load()) / 1e6
}

// LimitUSD returns the configured daily limit.
func (g *CostGuard) LimitUSD() float64 { return g.limitUSD }

func todayUTC() string {
	return time.Now().UTC().Format("2006-01-02")
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/infra/...
```

Expected: `PASS`

**Step 5: Commit**

```bash
git add internal/infra/cost.go internal/infra/cost_test.go
git commit -m "feat: add infra.CostGuard with atomic tracking, daily reset, and threshold notifications"
```

---

### Task 3: `internal/infra/soul.go`

**Files:**
- Create: `internal/infra/soul.go`
- Create: `internal/infra/soul_test.go`

**Step 1: Write the failing test**

```go
// internal/infra/soul_test.go
package infra_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/infra"
)

func TestSOULLoaderMissingFile(t *testing.T) {
	loader := infra.NewSOULLoader("/nonexistent/SOUL.md")
	content, err := loader.Load()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if content != "" {
		t.Errorf("expected empty content on error, got %q", content)
	}
}

func TestSOULLoaderLoadsContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SOUL.md")
	if err := os.WriteFile(path, []byte("you are a helpful assistant"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	loader := infra.NewSOULLoader(path)
	content, err := loader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if content != "you are a helpful assistant" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestSOULLoaderCachesOnMtime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SOUL.md")
	if err := os.WriteFile(path, []byte("v1"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	loader := infra.NewSOULLoader(path)
	content1, _ := loader.Load()

	// Call Load again without changing the file — mtime unchanged, cache must return v1.
	content2, _ := loader.Load()
	if content1 != content2 {
		t.Errorf("expected cached result; content changed without mtime change")
	}
}

func TestSOULLoaderReloadsOnMtimeChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SOUL.md")
	if err := os.WriteFile(path, []byte("v1"), 0600); err != nil {
		t.Fatalf("write v1: %v", err)
	}

	loader := infra.NewSOULLoader(path)
	content1, _ := loader.Load()
	if content1 != "v1" {
		t.Fatalf("initial load: got %q", content1)
	}

	// Ensure mtime changes (sleep 10ms to guarantee a different mtime on fast systems).
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("v2"), 0600); err != nil {
		t.Fatalf("write v2: %v", err)
	}

	content2, err := loader.Load()
	if err != nil {
		t.Fatalf("Load after update: %v", err)
	}
	if content2 != "v2" {
		t.Errorf("expected reload; got %q", content2)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/infra/... -run TestSOUL
```

Expected: `FAIL`

**Step 3: Write implementation**

```go
// internal/infra/soul.go
package infra

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// SOULLoader loads SOUL.md from disk with mtime-based caching.
// It reloads the file only when the modification time changes.
// Safe for concurrent use.
type SOULLoader struct {
	mu      sync.Mutex
	path    string
	content string
	mtime   time.Time
}

// NewSOULLoader creates a SOULLoader for the given file path.
func NewSOULLoader(path string) *SOULLoader {
	return &SOULLoader{path: path}
}

// Load returns the current content of SOUL.md.
// On first call, or when the file has been modified, it reads from disk.
// Returns an error if the file cannot be read.
func (l *SOULLoader) Load() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	info, err := os.Stat(l.path)
	if err != nil {
		return "", fmt.Errorf("soul: stat %q: %w", l.path, err)
	}

	// Use !l.mtime.IsZero() (not l.content != "") so an empty SOUL.md is cached correctly.
	if !l.mtime.IsZero() && info.ModTime().Equal(l.mtime) {
		return l.content, nil
	}

	data, err := os.ReadFile(l.path)
	if err != nil {
		return "", fmt.Errorf("soul: read %q: %w", l.path, err)
	}

	l.mtime = info.ModTime()
	l.content = string(data)
	return l.content, nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/infra/... -run TestSOUL
```

Expected: `PASS`

**Step 5: Commit**

```bash
git add internal/infra/soul.go internal/infra/soul_test.go
git commit -m "feat: add infra.SOULLoader with mtime-based caching"
```

---

### Task 4: `internal/infra/heartbeat.go`

**Files:**
- Create: `internal/infra/heartbeat.go`
- Create: `internal/infra/heartbeat_test.go`

**Step 1: Write the failing test**

```go
// internal/infra/heartbeat_test.go
package infra_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/canhta/gistclaw/internal/infra"
)

type mockHealthChecker struct {
	name  string
	alive bool
	calls atomic.Int32
}

func (m *mockHealthChecker) Name() string { return m.name }
func (m *mockHealthChecker) IsAlive(_ context.Context) bool {
	m.calls.Add(1)
	return m.alive
}

func TestNewHeartbeatNotNil(t *testing.T) {
	hb := infra.NewHeartbeat(nil, nil, 0)
	if hb == nil {
		t.Fatal("NewHeartbeat returned nil")
	}
}

func TestHeartbeatAgentHealthCheckerInterface(t *testing.T) {
	// Verify mockHealthChecker satisfies AgentHealthChecker.
	var _ infra.AgentHealthChecker = &mockHealthChecker{}
}

func TestHeartbeatCheckAgents(t *testing.T) {
	alive := &mockHealthChecker{name: "opencode", alive: true}
	dead := &mockHealthChecker{name: "claudecode", alive: false}

	hb := infra.NewHeartbeat(nil, []infra.AgentHealthChecker{alive, dead}, 0)

	results := hb.CheckAgents(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		switch r.Name {
		case "opencode":
			if !r.Alive {
				t.Error("opencode should be alive")
			}
		case "claudecode":
			if r.Alive {
				t.Error("claudecode should be dead")
			}
		}
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/infra/... -run TestHeartbeat -run TestNew
```

Expected: `FAIL`

**Step 3: Write implementation**

```go
// internal/infra/heartbeat.go
package infra

import (
	"context"
)

// AgentHealthChecker is implemented by opencode.Service and claudecode.Service.
type AgentHealthChecker interface {
	Name() string
	IsAlive(ctx context.Context) bool
}

// AgentHealthResult is the result of a single health check.
type AgentHealthResult struct {
	Name  string
	Alive bool
}

// Heartbeat owns Tier 1 (Telegram liveness) and Tier 2 (agent health) checks.
// The check methods are called by the service loops in app.Run — Heartbeat itself
// does not own any goroutine or ticker.
type Heartbeat struct {
	notifier   Notifier
	checkers   []AgentHealthChecker
	operatorID int64
}

// NewHeartbeat creates a Heartbeat. notifier may be nil.
func NewHeartbeat(notifier Notifier, checkers []AgentHealthChecker, operatorID int64) *Heartbeat {
	return &Heartbeat{
		notifier:   notifier,
		checkers:   checkers,
		operatorID: operatorID,
	}
}

// CheckAgents runs IsAlive on all registered checkers and returns results.
// This is called by the Tier 2 heartbeat loop in app.Run.
func (h *Heartbeat) CheckAgents(ctx context.Context) []AgentHealthResult {
	results := make([]AgentHealthResult, len(h.checkers))
	for i, c := range h.checkers {
		results[i] = AgentHealthResult{Name: c.Name(), Alive: c.IsAlive(ctx)}
	}
	return results
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/infra/...
```

Expected: `PASS` for all infra tests.

**Step 5: Run all tests**

```bash
go test ./...
```

Expected: `PASS` across all packages.

**Step 6: Commit**

```bash
git add internal/infra/heartbeat.go internal/infra/heartbeat_test.go
git commit -m "feat: add infra.Heartbeat with AgentHealthChecker interface and CheckAgents"
```

---

## Plan 2 complete

All store and infra packages are in place. Next plans (parallel): **Plan 3 — LLM Providers**, **Plan 4 — Tools & MCP**, **Plan 5 — Channel & HITL**.

### Deferred items

**Tier 1 heartbeat (Telegram `getMe` liveness):**
`Heartbeat` in this plan exposes only `CheckAgents()` for Tier 2. Tier 1 (`getMe` → 3 retries → `log.Fatal`) requires a Telegram client and must live in `internal/channel/telegram` or be driven from `internal/app`. It is explicitly deferred to the Channel & HITL plan (Plan 5) and the App wiring plan. Any plan implementing `internal/app/app.go` must include a Tier 1 goroutine that calls the Telegram gateway's health endpoint on `tuning.HeartbeatTier1Every` (default 30s) with 3 failure tolerance before `log.Fatal`.

**Job CRUD in `internal/store`:**
`sqlite.go` in this plan has no job table helpers (insert, list, update `next_run_at`, delete, enable/disable). These are required by `scheduler.Service` and must be added to `internal/store/sqlite.go` in the scheduler plan. The `jobs` table schema is already in `schema.sql` — only the Go helper methods are missing.

Final check:

```bash
go build ./...
go test ./...
```

Both should produce zero errors.
