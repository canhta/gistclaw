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
	"strings"
	"sync"
	"time"

	"github.com/canhta/gistclaw/internal/auth"
	telegramconnector "github.com/canhta/gistclaw/internal/connectors/telegram"
	whatsappconnector "github.com/canhta/gistclaw/internal/connectors/whatsapp"
	zalopersonalconnector "github.com/canhta/gistclaw/internal/connectors/zalopersonal"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/logstream"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	anthropicprov "github.com/canhta/gistclaw/internal/providers/anthropic"
	openaiprov "github.com/canhta/gistclaw/internal/providers/openai"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/runtime/capabilities"
	"github.com/canhta/gistclaw/internal/scheduler"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/teams"
	"github.com/canhta/gistclaw/internal/tools"
	"github.com/canhta/gistclaw/internal/web"
	shippedteams "github.com/canhta/gistclaw/teams"
)

type App struct {
	cfg              Config
	db               *store.DB
	convStore        *conversations.ConversationStore
	runtime          *runtime.Runtime
	scheduler        *scheduler.Service
	replay           *replay.Service
	logs             *logstream.Sink
	toolRegistry     *tools.Registry
	webServer        *web.Server
	connectors       []model.Connector
	supervisor       *connectorSupervisor
	supervisorConfig connectorSupervisorConfig
	toolCloser       io.Closer
	prepareMu        sync.Mutex
	prepared         bool
	webAddrMu        sync.RWMutex
	webAddress       string
	configPath       string
	binaryPath       string
	buildInfo        BuildInfo
	startedAt        time.Time
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
	if err := auth.EnsureSessionSecret(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := seedOperatorSettings(db, cfg); err != nil {
		_ = db.Close()
		return nil, err
	}

	project, err := ensureProjectState(context.Background(), db, cfg)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := syncOnboardingState(context.Background(), db, project); err != nil {
		_ = db.Close()
		return nil, err
	}

	teamDir, err := prepareTeamDir(cfg)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := validateTeamDir(teamDir); err != nil {
		_ = db.Close()
		return nil, err
	}

	convStore := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, convStore)
	capabilityRegistry := capabilities.NewRegistry()
	reg, toolCloser, err := buildToolRegistry(context.Background(), cfg, nil, capabilityRegistry)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	broadcaster := web.NewSSEBroadcaster()
	connectorNotifier := newConnectorRouteNotifier(db)
	rt := runtimeWiring(cfg, db, convStore, reg, capabilityRegistry, mem, newRunEventFanout(broadcaster, connectorNotifier))
	if err := rt.LoadBudgetSettings(context.Background()); err != nil {
		if toolCloser != nil {
			_ = toolCloser.Close()
		}
		_ = db.Close()
		return nil, fmt.Errorf("bootstrap: load budget settings: %w", err)
	}
	tools.RegisterCollaborationTools(reg, tools.CollaborationHandlers{
		DelegateTask: rt.DelegateTaskTool,
	})
	rt.SetStorageRoot(cfg.StorageRoot)
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
		cfg, err = resolveConnectorAgentIDs(cfg, snapshot)
		if err != nil {
			if toolCloser != nil {
				_ = toolCloser.Close()
			}
			_ = db.Close()
			return nil, fmt.Errorf("bootstrap: resolve connector agent ids: %w", err)
		}
	}
	rp := replayWiring(db)
	sched := scheduler.NewService(scheduler.NewStore(db), schedulerRuntimeDispatcher{runtime: rt})
	logs := logstream.New(500)

	var whatsappHealth *whatsappconnector.HealthState
	if cfg.WhatsApp.PhoneNumberID != "" || cfg.WhatsApp.AccessToken != "" || cfg.WhatsApp.VerifyToken != "" {
		whatsappHealth = whatsappconnector.NewHealthState(nil)
	}

	connectors := buildConnectors(cfg, db, convStore, rt, whatsappHealth)
	rt.SetConnectors(connectors)
	for _, connector := range connectors {
		capabilityRegistry.RegisterConnector(connector)
	}
	connectorNotifier.SetConnectors(connectors)

	application := &App{
		cfg:          cfg,
		db:           db,
		convStore:    convStore,
		runtime:      rt,
		scheduler:    sched,
		replay:       rp,
		logs:         logs,
		toolRegistry: reg,
		connectors:   connectors,
		toolCloser:   toolCloser,
		startedAt:    time.Now().UTC(),
	}
	capabilityRegistry.RegisterAppAction("status", application)

	webSrv, err := web.NewServer(web.Options{
		DB:              db,
		Replay:          rp,
		Broadcaster:     broadcaster,
		Runtime:         rt,
		Logs:            logs,
		Maintenance:     application,
		Nodes:           application,
		Schedules:       application,
		StorageRoot:     cfg.StorageRoot,
		WhatsAppWebhook: buildWhatsAppWebhook(cfg, db, convStore, rt, whatsappHealth),
		ConnectorHealth: application,
	})
	if err != nil {
		if toolCloser != nil {
			_ = toolCloser.Close()
		}
		_ = db.Close()
		return nil, fmt.Errorf("bootstrap: web server: %w", err)
	}
	application.webServer = webSrv
	return application, nil
}

func resolveConnectorAgentIDs(cfg Config, snapshot model.ExecutionSnapshot) (Config, error) {
	frontAgentID := strings.TrimSpace(snapshot.FrontAgentID)
	if frontAgentID == "" {
		return Config{}, fmt.Errorf("front_agent is required to resolve connector agent ids")
	}
	if cfg.Telegram.AgentID == "" {
		cfg.Telegram.AgentID = frontAgentID
	}
	if cfg.WhatsApp.AgentID == "" {
		cfg.WhatsApp.AgentID = frontAgentID
	}
	if cfg.ZaloPersonal.AgentID == "" {
		cfg.ZaloPersonal.AgentID = frontAgentID
	}
	return cfg, nil
}

func resolveTeamDir(cfg Config) string {
	if cfg.TeamDir != "" {
		return cfg.TeamDir
	}
	if cfg.StorageRoot == "" {
		return ""
	}
	return filepath.Join(cfg.StorageRoot, "teams", "default")
}

func prepareTeamDir(cfg Config) (string, error) {
	teamDir := resolveTeamDir(cfg)
	if teamDir == "" || cfg.TeamDir != "" {
		return teamDir, nil
	}

	if info, err := os.Stat(teamDir); err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("bootstrap: storage team dir %q is not a directory", teamDir)
		}
		return teamDir, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("bootstrap: stat storage team dir: %w", err)
	}

	if err := copyFS(shippedteams.Default(), teamDir); err != nil {
		return "", fmt.Errorf("bootstrap: seed shipped team dir: %w", err)
	}
	return teamDir, nil
}

func copyFS(src fs.FS, dstDir string) error {
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		targetPath := dstDir
		if path != "." {
			targetPath = filepath.Join(dstDir, path)
		}

		if d.IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", targetPath, err)
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("unsupported team entry %q", path)
		}

		data, err := fs.ReadFile(src, path)
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

func seedOperatorSettings(db *store.DB, cfg Config) error {
	if err := insertSettingIfMissing(db, "approval_mode", string(cfg.ApprovalMode)); err != nil {
		return fmt.Errorf("bootstrap: seed approval_mode: %w", err)
	}
	if err := insertSettingIfMissing(db, "host_access_mode", string(cfg.HostAccessMode)); err != nil {
		return fmt.Errorf("bootstrap: seed host_access_mode: %w", err)
	}
	if err := insertSettingIfMissing(db, "telegram_bot_token", cfg.Telegram.BotToken); err != nil {
		return fmt.Errorf("bootstrap: seed telegram_bot_token: %w", err)
	}
	return nil
}

func insertSettingIfMissing(db *store.DB, key, value string) error {
	if value == "" {
		return nil
	}
	_, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO NOTHING`,
		key,
		value,
	)
	if err != nil {
		return fmt.Errorf("insert %s: %w", key, err)
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
	capabilityRegistry *capabilities.Registry,
	mem *memory.Store,
	sink model.RunEventSink,
) *runtime.Runtime {
	prov := buildProvider(cfg.Provider)
	return runtime.New(db, cs, reg, capabilityRegistry, mem, prov, sink)
}

func ensureProjectState(ctx context.Context, db *store.DB, cfg Config) (model.Project, error) {
	activeProject, err := runtime.ActiveProject(ctx, db)
	if err != nil {
		return model.Project{}, fmt.Errorf("bootstrap: load active project: %w", err)
	}
	if activeProject.ID != "" {
		if err := runtime.SetActiveProject(ctx, db, activeProject.ID); err != nil {
			return model.Project{}, fmt.Errorf("bootstrap: sync active project: %w", err)
		}
		return activeProject, nil
	}
	project, err := createStarterProject(ctx, db)
	if err != nil {
		return model.Project{}, err
	}
	return project, nil
}

func syncOnboardingState(ctx context.Context, db *store.DB, project model.Project) error {
	if project.Source == "starter" {
		return nil
	}
	if _, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at)
		 VALUES ('onboarding_completed_at', datetime('now'), datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
	); err != nil {
		return fmt.Errorf("bootstrap: mark onboarding complete: %w", err)
	}
	return nil
}

func createStarterProject(ctx context.Context, db *store.DB) (model.Project, error) {
	projectsRoot := defaultProjectsRoot()
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		return model.Project{}, fmt.Errorf("bootstrap: create projects root: %w", err)
	}

	for attempt := 0; attempt < 32; attempt++ {
		name, err := generateStarterProjectName()
		if err != nil {
			return model.Project{}, fmt.Errorf("bootstrap: generate starter project name: %w", err)
		}
		projectPath := filepath.Join(projectsRoot, name)
		if err := os.Mkdir(projectPath, 0o755); err != nil {
			if os.IsExist(err) {
				continue
			}
			return model.Project{}, fmt.Errorf("bootstrap: create starter project path: %w", err)
		}
		project, err := runtime.ActivateProjectPath(ctx, db, projectPath, name, "starter")
		if err != nil {
			return model.Project{}, fmt.Errorf("bootstrap: activate starter project: %w", err)
		}
		return project, nil
	}

	return model.Project{}, fmt.Errorf("bootstrap: could not allocate a unique starter project name")
}

func generateStarterProjectName() (string, error) {
	adjectives := []string{"amber", "quiet", "silver", "steady", "bright", "cedar", "winter", "cinder"}
	nouns := []string{"fox", "harbor", "river", "forge", "meadow", "summit", "trail", "canvas"}

	adjective, err := randomWord(adjectives)
	if err != nil {
		return "", err
	}
	noun, err := randomWord(nouns)
	if err != nil {
		return "", err
	}

	suffix := make([]byte, 2)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%x", adjective, noun, suffix), nil
}

func randomWord(words []string) (string, error) {
	if len(words) == 0 {
		return "", fmt.Errorf("empty word list")
	}
	buf := make([]byte, 1)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return words[int(buf[0])%len(words)], nil
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

func buildToolRegistry(ctx context.Context, cfg Config, factory tools.MCPFactory, capabilityRegistry *capabilities.Registry) (*tools.Registry, io.Closer, error) {
	opts := tools.BuildOptions{
		Research:   cfg.Research,
		MCP:        cfg.MCP,
		MCPFactory: factory,
	}
	if capabilityRegistry != nil {
		opts.Capabilities = tools.CapabilityHandlers{
			InboxList:     capabilityRegistry.InboxList,
			InboxUpdate:   capabilityRegistry.InboxUpdate,
			DirectoryList: capabilityRegistry.DirectoryList,
			ResolveTarget: capabilityRegistry.ResolveTarget,
			Send:          capabilityRegistry.Send,
			Status:        capabilityRegistry.Status,
			AppAction:     capabilityRegistry.AppAction,
		}
	}
	return tools.BuildRegistry(ctx, opts)
}

func (a *App) WebAddress() string {
	a.webAddrMu.RLock()
	defer a.webAddrMu.RUnlock()
	return a.webAddress
}

func (a *App) LogWriter() io.Writer {
	if a == nil || a.logs == nil {
		return nil
	}
	return a.logs
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
	whatsappHealth *whatsappconnector.HealthState,
) []model.Connector {
	connectors := make([]model.Connector, 0, 3)
	if cfg.Telegram.BotToken != "" {
		connectors = append(connectors, telegramconnector.NewConnector(
			cfg.Telegram.BotToken,
			db,
			cs,
			rt,
			cfg.Telegram.AgentID,
		))
	}
	if cfg.WhatsApp.PhoneNumberID != "" && cfg.WhatsApp.AccessToken != "" {
		connectors = append(connectors, whatsappconnector.NewConnector(
			cfg.WhatsApp.PhoneNumberID,
			cfg.WhatsApp.AccessToken,
			db,
			cs,
			whatsappHealth,
		))
	}
	if cfg.ZaloPersonal.Enabled {
		connector := zalopersonalconnector.NewConnector(
			db,
			cs,
			rt,
			cfg.ZaloPersonal.AgentID,
		)
		allowlist := make(map[string]bool, len(cfg.ZaloPersonal.Groups.Allowlist))
		for _, groupID := range cfg.ZaloPersonal.Groups.Allowlist {
			groupID = strings.TrimSpace(groupID)
			if groupID == "" {
				continue
			}
			allowlist[groupID] = true
		}
		connector.SetGroupPolicy(zalopersonalconnector.GroupPolicy{
			Enabled:         cfg.ZaloPersonal.Groups.Enabled,
			Allowlist:       allowlist,
			MentionRequired: cfg.ZaloPersonal.Groups.ReplyMode != "open",
		})
		connectors = append(connectors, connector)
	}
	return connectors
}

func buildWhatsAppWebhook(
	cfg Config,
	db *store.DB,
	cs *conversations.ConversationStore,
	rt *runtime.Runtime,
	health *whatsappconnector.HealthState,
) http.Handler {
	if cfg.WhatsApp.VerifyToken == "" || cfg.WhatsApp.PhoneNumberID == "" || cfg.WhatsApp.AccessToken == "" {
		return nil
	}
	sender := whatsappconnector.NewOutboundDispatcher(
		cfg.WhatsApp.PhoneNumberID,
		cfg.WhatsApp.AccessToken,
		db,
		cs,
		health,
	)
	return whatsappconnector.NewWebhookHandler(
		cfg.WhatsApp.VerifyToken,
		cfg.WhatsApp.AgentID,
		rt,
		sender,
		health,
	)
}
