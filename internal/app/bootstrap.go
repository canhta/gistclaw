package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	telegramconnector "github.com/canhta/gistclaw/internal/connectors/telegram"
	whatsappconnector "github.com/canhta/gistclaw/internal/connectors/whatsapp"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	anthropicprov "github.com/canhta/gistclaw/internal/providers/anthropic"
	openaiprov "github.com/canhta/gistclaw/internal/providers/openai"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/teams"
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
	toolCloser io.Closer
	prepareMu  sync.Mutex
	prepared   bool
	webAddrMu  sync.RWMutex
	webAddress string
}

func Bootstrap(cfg Config) (*App, error) {
	teamDir, err := prepareTeamDir(cfg)
	if err != nil {
		return nil, err
	}

	db, err := storeWiring(cfg)
	if err != nil {
		return nil, err
	}

	if err := ensureAdminToken(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := validateTeamDir(teamDir); err != nil {
		_ = db.Close()
		return nil, err
	}

	convStore := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, convStore)
	reg, toolCloser, err := buildToolRegistry(context.Background(), cfg, nil)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	broadcaster := web.NewSSEBroadcaster()
	connectorNotifier := newConnectorRouteNotifier(db)
	rt := runtimeWiring(cfg, db, convStore, reg, mem, newRunEventFanout(broadcaster, connectorNotifier))
	rt.SetTeamDir(teamDir)
	if teamDir != "" {
		snapshot, err := teams.LoadExecutionSnapshot(teamDir)
		if err != nil {
			if toolCloser != nil {
				_ = toolCloser.Close()
			}
			_ = db.Close()
			return nil, fmt.Errorf("bootstrap: load execution snapshot: %w", err)
		}
		if err := rt.SetDefaultExecutionSnapshot(snapshot); err != nil {
			if toolCloser != nil {
				_ = toolCloser.Close()
			}
			_ = db.Close()
			return nil, fmt.Errorf("bootstrap: set execution snapshot: %w", err)
		}
	}
	rp := replayWiring(db)

	webSrv, err := web.NewServer(web.Options{
		DB:              db,
		Replay:          rp,
		Broadcaster:     broadcaster,
		Runtime:         rt,
		WhatsAppWebhook: buildWhatsAppWebhook(cfg, db, convStore, rt),
	})
	if err != nil {
		if toolCloser != nil {
			_ = toolCloser.Close()
		}
		_ = db.Close()
		return nil, fmt.Errorf("bootstrap: web server: %w", err)
	}

	connectors := buildConnectors(cfg, db, convStore, rt)
	connectorNotifier.SetConnectors(connectors)

	return &App{
		cfg:        cfg,
		db:         db,
		convStore:  convStore,
		runtime:    rt,
		replay:     rp,
		webServer:  webSrv,
		connectors: connectors,
		toolCloser: toolCloser,
	}, nil
}

func resolveTeamDir(cfg Config) string {
	if cfg.TeamDir != "" {
		return cfg.TeamDir
	}
	if cfg.WorkspaceRoot == "" {
		return ""
	}
	return filepath.Join(cfg.WorkspaceRoot, ".gistclaw", "teams", "default")
}

func prepareTeamDir(cfg Config) (string, error) {
	teamDir := resolveTeamDir(cfg)
	if teamDir == "" || cfg.TeamDir != "" {
		return teamDir, nil
	}

	if info, err := os.Stat(teamDir); err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("bootstrap: workspace team dir %q is not a directory", teamDir)
		}
		return teamDir, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("bootstrap: stat workspace team dir: %w", err)
	}

	sourceDir := filepath.Join(cfg.WorkspaceRoot, "teams", "default")
	info, err := os.Stat(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("bootstrap: stat source team dir: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("bootstrap: source team dir %q is not a directory", sourceDir)
	}
	if err := copyDirectory(sourceDir, teamDir); err != nil {
		return "", fmt.Errorf("bootstrap: seed workspace team dir: %w", err)
	}
	return teamDir, nil
}

func copyDirectory(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		targetPath := filepath.Join(dstDir, relPath)

		if d.IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", targetPath, err)
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("unsupported team entry %q", path)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(targetPath), err)
		}
		if err := os.WriteFile(targetPath, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", targetPath, err)
		}
		return nil
	})
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
		return openaiprov.New(cfg.APIKey, modelID, cfg.BaseURL, cfg.WireAPI)
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

func buildToolRegistry(ctx context.Context, cfg Config, factory tools.MCPFactory) (*tools.Registry, io.Closer, error) {
	return tools.BuildRegistry(ctx, tools.BuildOptions{
		Research:   cfg.Research,
		MCP:        cfg.MCP,
		MCPFactory: factory,
	})
}

func (a *App) WebAddress() string {
	a.webAddrMu.RLock()
	defer a.webAddrMu.RUnlock()
	return a.webAddress
}

func (a *App) setWebAddress(addr string) {
	a.webAddrMu.Lock()
	defer a.webAddrMu.Unlock()
	a.webAddress = addr
}

func buildConnectors(
	cfg Config,
	db *store.DB,
	cs *conversations.ConversationStore,
	rt *runtime.Runtime,
) []model.Connector {
	connectors := make([]model.Connector, 0, 2)
	if cfg.Telegram.BotToken != "" {
		connectors = append(connectors, telegramconnector.NewConnector(
			cfg.Telegram.BotToken,
			db,
			cs,
			rt,
			cfg.Telegram.AgentID,
			cfg.WorkspaceRoot,
		))
	}
	if cfg.WhatsApp.PhoneNumberID != "" && cfg.WhatsApp.AccessToken != "" {
		connectors = append(connectors, whatsappconnector.NewConnector(
			cfg.WhatsApp.PhoneNumberID,
			cfg.WhatsApp.AccessToken,
			db,
			cs,
		))
	}
	return connectors
}

func buildWhatsAppWebhook(cfg Config, db *store.DB, cs *conversations.ConversationStore, rt *runtime.Runtime) http.Handler {
	if cfg.WhatsApp.VerifyToken == "" || cfg.WhatsApp.PhoneNumberID == "" || cfg.WhatsApp.AccessToken == "" {
		return nil
	}
	sender := whatsappconnector.NewOutboundDispatcher(
		cfg.WhatsApp.PhoneNumberID,
		cfg.WhatsApp.AccessToken,
		db,
		cs,
	)
	return whatsappconnector.NewWebhookHandler(
		cfg.WhatsApp.VerifyToken,
		cfg.WhatsApp.AgentID,
		cfg.WorkspaceRoot,
		rt,
		sender,
	)
}
