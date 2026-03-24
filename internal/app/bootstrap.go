package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
	"github.com/canhta/gistclaw/internal/web"
)

type App struct {
	cfg       Config
	db        *store.DB
	convStore *conversations.ConversationStore
	runtime   *runtime.Runtime
	replay    *replay.Service
	webServer *web.Server
}

func Bootstrap(cfg Config) (*App, error) {
	db, err := storeWiring(cfg)
	if err != nil {
		return nil, err
	}

	if err := ensureAdminToken(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	convStore := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, convStore)
	reg := tools.NewRegistry()

	broadcaster := web.NewSSEBroadcaster()
	rt := runtimeWiring(cfg, db, convStore, reg, mem, broadcaster)
	rp := replayWiring(db)

	webSrv, err := web.NewServer(web.Options{
		DB:          db,
		Replay:      rp,
		Broadcaster: broadcaster,
		Runtime:     rt,
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("bootstrap: web server: %w", err)
	}

	return &App{
		cfg:       cfg,
		db:        db,
		convStore: convStore,
		runtime:   rt,
		replay:    rp,
		webServer: webSrv,
	}, nil
}

// ensureAdminToken generates and persists a cryptographically random 32-byte
// admin token if one does not already exist in the settings table.
// The token is never logged at info level.
func ensureAdminToken(db *store.DB) error {
	var existing string
	err := db.RawDB().QueryRow("SELECT value FROM settings WHERE key = 'admin_token'").Scan(&existing)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("bootstrap: check admin token: %w", err)
	}
	if existing != "" {
		return nil
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("bootstrap: generate admin token: %w", err)
	}
	token := hex.EncodeToString(buf)

	_, err = db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES ('admin_token', ?, datetime('now'))
		 ON CONFLICT(key) DO NOTHING`,
		token,
	)
	if err != nil {
		return fmt.Errorf("bootstrap: store admin token: %w", err)
	}
	return nil
}

func storeWiring(cfg Config) (*store.DB, error) {
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func runtimeWiring(
	_ Config,
	db *store.DB,
	cs *conversations.ConversationStore,
	reg *tools.Registry,
	mem *memory.Store,
	sink model.RunEventSink,
) *runtime.Runtime {
	prov := runtime.NewMockProvider(nil, nil)
	return runtime.New(db, cs, reg, mem, prov, sink)
}

func replayWiring(db *store.DB) *replay.Service {
	return replay.NewService(db)
}
