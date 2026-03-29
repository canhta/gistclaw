package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/scheduler"
	"github.com/canhta/gistclaw/internal/store"
)

type Options struct {
	DB              *store.DB
	Replay          *replay.Service
	Broadcaster     *SSEBroadcaster
	Runtime         *runtime.Runtime
	Schedules       scheduleService
	StorageRoot     string
	WhatsAppWebhook http.Handler
	ConnectorHealth connectorHealthSource
}

type Server struct {
	db              *store.DB
	replay          *replay.Service
	broadcaster     *SSEBroadcaster
	rt              *runtime.Runtime
	schedules       scheduleService
	storageRoot     string
	whatsAppWebhook http.Handler
	connectorHealth connectorHealthSource
	mux             *http.ServeMux
}

type connectorHealthSource interface {
	ConnectorHealth(context.Context) ([]model.ConnectorHealthSnapshot, error)
}

type scheduleService interface {
	CreateSchedule(context.Context, scheduler.CreateScheduleInput) (scheduler.Schedule, error)
	UpdateSchedule(context.Context, string, scheduler.UpdateScheduleInput) (scheduler.Schedule, error)
	ListSchedules(context.Context) ([]scheduler.Schedule, error)
	LoadSchedule(context.Context, string) (scheduler.Schedule, error)
	EnableSchedule(context.Context, string) (scheduler.Schedule, error)
	DisableSchedule(context.Context, string) (scheduler.Schedule, error)
	DeleteSchedule(context.Context, string) error
	ScheduleStatus(context.Context) (scheduler.StatusSummary, error)
	RunScheduleNow(context.Context, string) (*scheduler.ClaimedOccurrence, error)
}

type shellProjectLayout struct {
	ActiveName        string
	ActiveProjectPath string
	Options           []shellProjectOption
}

type shellProjectOption struct {
	ID     string
	Name   string
	Active bool
}

func NewServer(opts Options) (*Server, error) {
	if opts.DB == nil {
		return nil, fmt.Errorf("web: db is required")
	}
	if opts.Replay == nil {
		return nil, fmt.Errorf("web: replay service is required")
	}
	if opts.Broadcaster == nil {
		opts.Broadcaster = NewSSEBroadcaster()
	}

	s := &Server{
		db:              opts.DB,
		replay:          opts.Replay,
		broadcaster:     opts.Broadcaster,
		rt:              opts.Runtime,
		schedules:       opts.Schedules,
		storageRoot:     opts.StorageRoot,
		whatsAppWebhook: opts.WhatsAppWebhook,
		connectorHealth: opts.ConnectorHealth,
		mux:             http.NewServeMux(),
	}
	s.registerRoutes()

	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := s.recoverMiddleware(s.requestLogger(s.authGate(s.onboardingMiddleware(s.mux))))
	handler.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	spaAssets := serveSPAAssets()
	s.mux.HandleFunc("GET /{$}", s.handleSPADocument)
	s.mux.Handle("GET /_app/{path...}", spaAssets)
	s.mux.Handle("GET /robots.txt", spaAssets)
	s.mux.HandleFunc("GET "+pageLogin, s.handleLogin)
	s.mux.HandleFunc("GET "+pageLogout, s.handleLogout)
	s.mux.HandleFunc("POST "+pageLogout, s.handleLogout)
	s.mux.HandleFunc("GET "+pageOnboarding, s.handleSPADocument)
	s.mux.HandleFunc("GET "+pageWork, s.handleSPADocument)
	s.mux.HandleFunc("GET "+pageWork+"/{id}", s.handleSPADocument)
	s.mux.HandleFunc("GET "+pageTeam, s.handleSPADocument)
	s.mux.HandleFunc("GET "+pageKnowledge, s.handleSPADocument)
	s.mux.HandleFunc("GET "+pageRecover, s.handleSPADocument)
	s.mux.HandleFunc("GET "+pageConversations, s.handleSPADocument)
	s.mux.HandleFunc("GET "+pageConversations+"/{id}", s.handleSPADocument)
	s.mux.HandleFunc("GET "+pageAutomate, s.handleSPADocument)
	s.mux.HandleFunc("GET "+pageHistory, s.handleSPADocument)
	s.mux.HandleFunc("GET "+pageSettings, s.handleSPADocument)
	s.mux.HandleFunc("GET /api/auth/session", s.handleAuthSession)
	s.mux.HandleFunc("POST /api/auth/login", s.handleAuthLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
	s.mux.HandleFunc("GET /api/bootstrap", s.handleBootstrap)
	s.mux.HandleFunc("GET /api/onboarding", s.handleOnboardingAPI)
	s.mux.Handle("POST /api/onboarding/project", s.adminAuth(http.HandlerFunc(s.handleOnboardingProjectAPI)))
	s.mux.Handle("POST /api/onboarding/preview", s.adminAuth(http.HandlerFunc(s.handleOnboardingPreviewAPI)))
	s.mux.HandleFunc("GET /api/work", s.handleWorkIndex)
	s.mux.Handle("POST /api/work", s.adminAuth(http.HandlerFunc(s.handleWorkCreate)))
	s.mux.HandleFunc("GET /api/work/{id}", s.handleWorkDetail)
	s.mux.HandleFunc("GET /api/work/{id}/graph", s.handleRunGraph)
	s.mux.HandleFunc("GET /api/work/{id}/nodes/{node_id}", s.handleRunNodeDetail)
	s.mux.HandleFunc("GET /api/work/{id}/events", s.handleRunEvents)
	s.mux.Handle("POST /api/work/{id}/dismiss", s.adminAuth(http.HandlerFunc(s.handleWorkDismiss)))
	s.mux.HandleFunc("GET /api/team", s.handleTeamAPI)
	s.mux.HandleFunc("GET /api/team/export", s.handleTeamExportAPI)
	s.mux.Handle("POST /api/team/select", s.adminAuth(http.HandlerFunc(s.handleTeamSelectAPI)))
	s.mux.Handle("POST /api/team/create", s.adminAuth(http.HandlerFunc(s.handleTeamCreateAPI)))
	s.mux.Handle("POST /api/team/clone", s.adminAuth(http.HandlerFunc(s.handleTeamCloneAPI)))
	s.mux.Handle("POST /api/team/delete", s.adminAuth(http.HandlerFunc(s.handleTeamDeleteAPI)))
	s.mux.Handle("POST /api/team/save", s.adminAuth(http.HandlerFunc(s.handleTeamSaveAPI)))
	s.mux.Handle("POST /api/team/import", s.adminAuth(http.HandlerFunc(s.handleTeamImportAPI)))
	s.mux.HandleFunc("GET /api/knowledge", s.handleKnowledgeIndex)
	s.mux.Handle("POST /api/knowledge/{id}/edit", s.adminAuth(http.HandlerFunc(s.handleKnowledgeEdit)))
	s.mux.Handle("POST /api/knowledge/{id}/forget", s.adminAuth(http.HandlerFunc(s.handleKnowledgeForget)))
	s.mux.HandleFunc("GET /api/recover", s.handleRecoverIndex)
	s.mux.Handle("POST /api/recover/approvals/{id}/resolve", s.adminAuth(http.HandlerFunc(s.handleApprovalResolve)))
	s.mux.Handle("POST /api/recover/routes/{id}/messages", s.adminAuth(http.HandlerFunc(s.handleRouteSend)))
	s.mux.Handle("POST /api/recover/routes/{id}/deactivate", s.adminAuth(http.HandlerFunc(s.handleRouteDeactivate)))
	s.mux.Handle("POST /api/recover/deliveries/{id}/retry", s.adminAuth(http.HandlerFunc(s.handleDeliveryRetry)))
	s.mux.HandleFunc("GET /api/conversations", s.handleConversationsIndex)
	s.mux.HandleFunc("GET /api/conversations/{id}", s.handleConversationDetail)
	s.mux.Handle("POST /api/conversations/{id}/messages", s.adminAuth(http.HandlerFunc(s.handleSessionSend)))
	s.mux.Handle("POST /api/conversations/{id}/deliveries/{delivery_id}/retry", s.adminAuth(http.HandlerFunc(s.handleSessionRetryDelivery)))
	s.mux.HandleFunc("GET /api/automate", s.handleAutomateIndex)
	s.mux.Handle("POST /api/automate", s.adminAuth(http.HandlerFunc(s.handleAutomateCreate)))
	s.mux.Handle("POST /api/automate/{id}/enable", s.adminAuth(http.HandlerFunc(s.handleAutomateEnable)))
	s.mux.Handle("POST /api/automate/{id}/disable", s.adminAuth(http.HandlerFunc(s.handleAutomateDisable)))
	s.mux.Handle("POST /api/automate/{id}/run", s.adminAuth(http.HandlerFunc(s.handleAutomateRun)))
	s.mux.HandleFunc("GET /api/history", s.handleHistoryIndex)
	s.mux.HandleFunc("GET /api/settings", s.handleSettingsAPI)
	s.mux.Handle("POST /api/settings", s.adminAuth(http.HandlerFunc(s.handleSettingsUpdateAPI)))
	s.mux.Handle("POST /api/settings/password", s.adminAuth(http.HandlerFunc(s.handleSettingsPasswordChangeAPI)))
	s.mux.Handle("POST /api/settings/devices/{id}/revoke", s.adminAuth(http.HandlerFunc(s.handleSettingsDeviceRevokeAPI)))
	s.mux.Handle("POST /api/settings/devices/{id}/block", s.adminAuth(http.HandlerFunc(s.handleSettingsDeviceBlockAPI)))
	s.mux.Handle("POST /api/settings/devices/{id}/unblock", s.adminAuth(http.HandlerFunc(s.handleSettingsDeviceUnblockAPI)))
	if s.whatsAppWebhook != nil {
		s.mux.Handle("GET /webhooks/whatsapp", s.whatsAppWebhook)
		s.mux.Handle("POST /webhooks/whatsapp", s.whatsAppWebhook)
	} else {
		s.mux.HandleFunc("GET /webhooks/whatsapp", http.NotFound)
		s.mux.HandleFunc("POST /webhooks/whatsapp", http.NotFound)
	}
	s.mux.Handle("POST /api/routes", s.adminAuth(http.HandlerFunc(s.handleRouteCreate)))
	s.mux.HandleFunc("GET /api/routes", s.handleRoutesIndex)
	s.mux.Handle("POST /api/routes/{id}/messages", s.adminAuth(http.HandlerFunc(s.handleRouteSend)))
	s.mux.Handle("POST /api/routes/{id}/deactivate", s.adminAuth(http.HandlerFunc(s.handleRouteDeactivate)))
	s.mux.HandleFunc("GET /api/deliveries", s.handleDeliveryIndex)
	s.mux.HandleFunc("GET /api/deliveries/health", s.handleDeliveryHealth)
	s.mux.Handle("POST /api/deliveries/{id}/retry", s.adminAuth(http.HandlerFunc(s.handleDeliveryRetry)))
	s.mux.Handle("POST /projects/activate", s.adminAuth(http.HandlerFunc(s.handleProjectActivate)))
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runID := runIDFromRequest(r)
		if runID != "" {
			log.Printf("web request method=%s path=%s run_id=%s", r.Method, r.URL.Path, runID)
		} else {
			log.Printf("web request method=%s path=%s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("web panic method=%s path=%s err=%v", r.Method, r.URL.Path, rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) projectLayoutData(r *http.Request) (shellProjectLayout, error) {
	project, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		return shellProjectLayout{}, fmt.Errorf("load active project: %w", err)
	}
	projects, err := runtime.ListProjects(r.Context(), s.db)
	if err != nil {
		return shellProjectLayout{}, fmt.Errorf("list projects: %w", err)
	}

	layout := shellProjectLayout{
		ActiveName:        project.Name,
		ActiveProjectPath: project.PrimaryPath,
	}
	for _, candidate := range projects {
		if candidate.ID == "" {
			continue
		}
		layout.Options = append(layout.Options, shellProjectOption{
			ID:     candidate.ID,
			Name:   candidate.Name,
			Active: candidate.ID == project.ID || (project.ID == "" && candidate.PrimaryPath == project.PrimaryPath),
		})
	}
	return layout, nil
}

func requestPathWithQuery(r *http.Request) string {
	if r == nil || r.URL == nil {
		return pageWork
	}
	if raw := r.URL.RequestURI(); raw != "" {
		return raw
	}
	return r.URL.Path
}

func (s *Server) authorizedByBearer(r *http.Request, adminToken string) bool {
	return r.Header.Get("Authorization") == "Bearer "+adminToken
}

func sameOriginRequest(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return sameRequestHost(origin, requestOriginHost(r))
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		return sameRequestHost(referer, requestOriginHost(r))
	}
	return false
}

func sameRequestHost(rawURL, wantHost string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, wantHost)
}

func (s *Server) writeForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("forbidden"))
}

func (s *Server) writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("unauthorized"))
}

func lookupSetting(db *store.DB, key string) string {
	var value string
	err := db.RawDB().QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ""
		}
		return ""
	}
	return value
}

func runIDFromRequest(r *http.Request) string {
	if runID := r.PathValue("id"); runID != "" {
		return runID
	}
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && parts[0] == "runs" {
		return parts[1]
	}
	return ""
}
