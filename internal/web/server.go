package web

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
)

const hostAdminCookieName = "gistclaw_admin"

type Options struct {
	DB              *store.DB
	Replay          *replay.Service
	Broadcaster     *SSEBroadcaster
	Runtime         *runtime.Runtime
	WhatsAppWebhook http.Handler
	ConnectorHealth connectorHealthSource
}

type Server struct {
	db              *store.DB
	replay          *replay.Service
	broadcaster     *SSEBroadcaster
	rt              *runtime.Runtime
	whatsAppWebhook http.Handler
	connectorHealth connectorHealthSource
	templates       *template.Template
	mux             *http.ServeMux
}

type connectorHealthSource interface {
	ConnectorHealth(context.Context) ([]model.ConnectorHealthSnapshot, error)
}

type layoutData struct {
	Title       string
	Body        template.HTML
	CurrentPath string
	Navigation  shellNavigation
	Project     shellProjectLayout
	ShellMode   string
	MainClass   string
}

type shellProjectLayout struct {
	ActiveName          string
	ActiveWorkspaceRoot string
	Options             []shellProjectOption
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

	tpls, err := loadTemplates()
	if err != nil {
		return nil, err
	}

	s := &Server{
		db:              opts.DB,
		replay:          opts.Replay,
		broadcaster:     opts.Broadcaster,
		rt:              opts.Runtime,
		whatsAppWebhook: opts.WhatsAppWebhook,
		connectorHealth: opts.ConnectorHealth,
		templates:       tpls,
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
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, pageOperateRuns, http.StatusSeeOther)
	})
	s.mux.HandleFunc("GET /operate", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, pageOperateRuns, http.StatusSeeOther)
	})
	s.mux.HandleFunc("GET /configure", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, pageConfigureTeam, http.StatusSeeOther)
	})
	s.mux.HandleFunc("GET /recover", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, pageRecoverApprovals, http.StatusSeeOther)
	})
	s.mux.Handle("GET /assets/{path...}", http.StripPrefix("/assets/", http.FileServer(http.FS(staticAssetFS()))))
	s.mux.HandleFunc("GET "+pageLogin, s.handleLogin)
	s.mux.HandleFunc("POST "+pageLogin, s.handleLoginSubmit)
	s.mux.HandleFunc("POST "+pageLogout, s.handleLogout)
	if s.whatsAppWebhook != nil {
		s.mux.Handle("GET /webhooks/whatsapp", s.whatsAppWebhook)
		s.mux.Handle("POST /webhooks/whatsapp", s.whatsAppWebhook)
	} else {
		s.mux.HandleFunc("GET /webhooks/whatsapp", http.NotFound)
		s.mux.HandleFunc("POST /webhooks/whatsapp", http.NotFound)
	}
	s.mux.HandleFunc("GET /api/sessions", s.handleSessionsIndex)
	s.mux.HandleFunc("GET /api/sessions/{id}", s.handleSessionDetail)
	s.mux.HandleFunc("GET "+pageOperateSessions, s.handleSessionPageIndex)
	s.mux.HandleFunc("GET "+pageOperateSessions+"/{id}", s.handleSessionPageDetail)
	s.mux.Handle("POST "+pageOperateSessions+"/{id}/messages", s.adminAuth(http.HandlerFunc(s.handleSessionPageSend)))
	s.mux.Handle("POST "+pageOperateSessions+"/{id}/deliveries/{delivery_id}/retry", s.adminAuth(http.HandlerFunc(s.handleSessionPageRetryDelivery)))
	s.mux.HandleFunc("GET "+pageRecoverRoutesDeliveries, s.handleRoutesDeliveriesPage)
	s.mux.HandleFunc("GET "+pageConfigureTeam, s.handleTeam)
	s.mux.HandleFunc("GET "+pageConfigureTeamExport, s.handleTeamExport)
	s.mux.Handle("POST "+pageConfigureTeam, s.adminAuth(http.HandlerFunc(s.handleTeamUpdate)))
	s.mux.Handle("POST "+pageRecoverRoutesDeliveries+"/routes/{id}/messages", s.adminAuth(http.HandlerFunc(s.handleRoutesDeliveriesRouteSend)))
	s.mux.Handle("POST "+pageRecoverRoutesDeliveries+"/routes/{id}/deactivate", s.adminAuth(http.HandlerFunc(s.handleRoutesDeliveriesRouteDeactivate)))
	s.mux.Handle("POST "+pageRecoverRoutesDeliveries+"/deliveries/{id}/retry", s.adminAuth(http.HandlerFunc(s.handleRoutesDeliveriesDeliveryRetry)))
	s.mux.Handle("POST /api/routes", s.adminAuth(http.HandlerFunc(s.handleRouteCreate)))
	s.mux.HandleFunc("GET /api/routes", s.handleRoutesIndex)
	s.mux.Handle("POST /api/routes/{id}/messages", s.adminAuth(http.HandlerFunc(s.handleRouteSend)))
	s.mux.Handle("POST /api/routes/{id}/deactivate", s.adminAuth(http.HandlerFunc(s.handleRouteDeactivate)))
	s.mux.HandleFunc("GET /api/deliveries", s.handleDeliveryIndex)
	s.mux.HandleFunc("GET /api/deliveries/health", s.handleDeliveryHealth)
	s.mux.Handle("POST /api/deliveries/{id}/retry", s.adminAuth(http.HandlerFunc(s.handleDeliveryRetry)))
	s.mux.Handle("POST /api/sessions/{id}/messages", s.adminAuth(http.HandlerFunc(s.handleSessionSend)))
	s.mux.Handle("POST /api/sessions/{id}/deliveries/{delivery_id}/retry", s.adminAuth(http.HandlerFunc(s.handleSessionRetryDelivery)))
	s.mux.HandleFunc("GET "+pageOperateRuns, s.handleRunsIndex)
	s.mux.HandleFunc("GET "+pageOperateRuns+"/{id}", s.handleRunDetail)
	s.mux.HandleFunc("GET "+pageOperateRuns+"/{id}/graph", s.handleRunGraph)
	s.mux.HandleFunc("GET "+pageOperateRuns+"/{id}/nodes/{node_id}", s.handleRunNodeDetail)
	s.mux.HandleFunc("GET "+pageOperateRuns+"/{id}/events", s.handleRunEvents)
	s.mux.Handle("POST "+pageOperateRuns+"/{id}/dismiss", s.adminAuth(http.HandlerFunc(s.handleRunDismiss)))
	s.mux.HandleFunc("GET "+pageRecoverApprovals, s.handleApprovals)
	s.mux.Handle("POST "+pageRecoverApprovals+"/{id}/resolve", s.adminAuth(http.HandlerFunc(s.handleApprovalResolve)))
	s.mux.HandleFunc("GET "+pageConfigureSettings, s.handleSettings)
	s.mux.Handle("POST "+pageConfigureSettings, s.adminAuth(http.HandlerFunc(s.handleSettingsUpdate)))
	s.mux.Handle("POST "+pageConfigureSettingsPassword, s.adminAuth(http.HandlerFunc(s.handleSettingsPasswordChange)))
	s.mux.Handle("POST "+pageConfigureSettingsDevices+"/{id}/revoke", s.adminAuth(http.HandlerFunc(s.handleDeviceRevoke)))
	s.mux.Handle("POST "+pageConfigureSettingsDevices+"/{id}/block", s.adminAuth(http.HandlerFunc(s.handleDeviceBlock)))
	s.mux.Handle("POST "+pageConfigureSettingsDevices+"/{id}/unblock", s.adminAuth(http.HandlerFunc(s.handleDeviceUnblock)))
	s.mux.HandleFunc("GET "+pageOperateStartTask, s.handleRunForm)
	s.mux.Handle("POST "+pageOperateStartTask, s.adminAuth(http.HandlerFunc(s.handleRunSubmit)))
	s.mux.Handle("POST /projects/activate", s.adminAuth(http.HandlerFunc(s.handleProjectActivate)))
	s.mux.HandleFunc("GET /onboarding", s.handleOnboarding)
	s.mux.HandleFunc("POST /onboarding", s.handleOnboardingStep1Submit)
	s.mux.HandleFunc("GET /onboarding/step/2", s.handleOnboardingStep2)
	s.mux.HandleFunc("GET /onboarding/step/3", s.handleOnboardingStep3)
	s.mux.HandleFunc("POST /onboarding/step/3", s.handleOnboardingStep3Submit)
	s.mux.HandleFunc("GET /onboarding/step/4/{id}", s.handleOnboardingStep4)
	s.mux.HandleFunc("GET "+pageConfigureMemory, s.handleMemoryList)
	s.mux.Handle("POST "+pageConfigureMemory+"/{id}/forget", s.adminAuth(http.HandlerFunc(s.handleMemoryForget)))
	s.mux.Handle("POST "+pageConfigureMemory+"/{id}/edit", s.adminAuth(http.HandlerFunc(s.handleMemoryEdit)))
}

func (s *Server) renderTemplate(w http.ResponseWriter, r *http.Request, title, bodyTemplate string, data any) {
	s.renderTemplateStatusMode(w, r, http.StatusOK, title, bodyTemplate, data, shellModeApp)
}

func (s *Server) renderTemplateStatus(w http.ResponseWriter, r *http.Request, status int, title, bodyTemplate string, data any) {
	s.renderTemplateStatusMode(w, r, status, title, bodyTemplate, data, shellModeApp)
}

func (s *Server) renderAuthTemplate(w http.ResponseWriter, r *http.Request, title, bodyTemplate string, data any) {
	s.renderTemplateStatusMode(w, r, http.StatusOK, title, bodyTemplate, data, shellModeAuth)
}

func (s *Server) renderAuthTemplateStatus(w http.ResponseWriter, r *http.Request, status int, title, bodyTemplate string, data any) {
	s.renderTemplateStatusMode(w, r, status, title, bodyTemplate, data, shellModeAuth)
}

func (s *Server) renderTemplateStatusMode(w http.ResponseWriter, r *http.Request, status int, title, bodyTemplate string, data any, mode string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var body bytes.Buffer
	if err := s.templates.ExecuteTemplate(&body, bodyTemplate, data); err != nil {
		http.Error(w, fmt.Sprintf("template render failed: %v", err), http.StatusInternalServerError)
		return
	}
	projectLayout := shellProjectLayout{}
	navigation := shellNavigation{}
	if mode == shellModeApp {
		var err error
		projectLayout, err = s.projectLayoutData(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("template render failed: %v", err), http.StatusInternalServerError)
			return
		}
		navigation = navigationForPath(r.URL.Path)
	}
	w.WriteHeader(status)
	if err := s.templates.ExecuteTemplate(w, "layout", layoutData{
		Title:       title,
		Body:        template.HTML(body.String()),
		CurrentPath: requestPathWithQuery(r),
		Navigation:  navigation,
		Project:     projectLayout,
		ShellMode:   mode,
		MainClass:   layoutMainClass(mode),
	}); err != nil {
		http.Error(w, fmt.Sprintf("template render failed: %v", err), http.StatusInternalServerError)
	}
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
		ActiveName:          project.Name,
		ActiveWorkspaceRoot: project.PrimaryPath,
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
		return pageOperateRuns
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
		return sameRequestHost(origin, r.Host)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		return sameRequestHost(referer, r.Host)
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

func layoutMainClass(mode string) string {
	if mode == shellModeAuth {
		return "auth-main"
	}
	return ""
}
