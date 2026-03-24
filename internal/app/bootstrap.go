package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/canhta/gistclaw/internal/connectors/email"
	"github.com/canhta/gistclaw/internal/connectors/telegram"
	"github.com/canhta/gistclaw/internal/connectors/whatsapp"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	anthropicprov "github.com/canhta/gistclaw/internal/providers/anthropic"
	openaiprov "github.com/canhta/gistclaw/internal/providers/openai"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/scheduler"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
	"github.com/canhta/gistclaw/internal/web"
)

type App struct {
	cfg        Config
	db         *store.DB
	convStore  *conversations.ConversationStore
	runtime    *runtime.Runtime
	replay     *replay.Service
	webServer  *web.Server
	connectors []model.Connector
	scheduler  *scheduler.Dispatcher
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

	if err := validateTeamDir(cfg.TeamDir); err != nil {
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

	// Wire connectors declared in settings.
	var connectors []model.Connector
	if tgToken := lookupDBSetting(db, "telegram_bot_token"); tgToken != "" {
		connectors = append(connectors, telegram.NewOutboundDispatcher(tgToken, db, convStore))
	}
	if phoneID := lookupDBSetting(db, "whatsapp_phone_number_id"); phoneID != "" {
		token := lookupDBSetting(db, "whatsapp_access_token")
		connectors = append(connectors, whatsapp.NewOutboundDispatcher(phoneID, token, db, convStore))
	}
	if smtpAddr := lookupDBSetting(db, "email_smtp_addr"); smtpAddr != "" {
		connectors = append(connectors, email.NewOutboundDispatcher(email.SMTPConfig{
			Addr:     smtpAddr,
			From:     lookupDBSetting(db, "email_from"),
			Username: lookupDBSetting(db, "email_smtp_username"),
			Password: lookupDBSetting(db, "email_smtp_password"),
		}, db, convStore))
	}

	sched := scheduler.NewDispatcher(db, convStore, rt, cfg.WorkspaceRoot)

	return &App{
		cfg:        cfg,
		db:         db,
		convStore:  convStore,
		runtime:    rt,
		replay:     rp,
		webServer:  webSrv,
		connectors: connectors,
		scheduler:  sched,
	}, nil
}

// lookupDBSetting reads a single setting value from the database.
func lookupDBSetting(db *store.DB, key string) string {
	var value string
	_ = db.RawDB().QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	return value
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
	cfg Config,
	db *store.DB,
	cs *conversations.ConversationStore,
	reg *tools.Registry,
	mem *memory.Store,
	sink model.RunEventSink,
) *runtime.Runtime {
	prov := buildProvider(cfg.Provider)
	return runtime.New(db, cs, reg, mem, prov, sink)
}

// buildProvider instantiates the correct runtime.Provider from the config.
// Supports "anthropic" (default) and any OpenAI-compatible provider ("openai").
// The base_url field allows pointing at alternative endpoints (Ollama, Groq,
// Azure OpenAI, LM Studio, Together AI, etc.).
func buildProvider(cfg ProviderConfig) runtime.Provider {
	modelID := cfg.Models.Strong
	switch cfg.Name {
	case "openai":
		if modelID == "" {
			modelID = "gpt-4o"
		}
		return openaiprov.New(cfg.APIKey, modelID, cfg.BaseURL)
	default: // "anthropic" and anything unrecognised falls back to Anthropic
		if modelID == "" {
			modelID = "claude-3-5-sonnet-20241022"
		}
		return anthropicprov.New(cfg.APIKey, modelID)
	}
}

func replayWiring(db *store.DB) *replay.Service {
	return replay.NewService(db)
}
