package web

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
)

type Options struct {
	DB          *store.DB
	Replay      *replay.Service
	Broadcaster *SSEBroadcaster
	Runtime     *runtime.Runtime
}

type Server struct {
	db          *store.DB
	replay      *replay.Service
	broadcaster *SSEBroadcaster
	rt          *runtime.Runtime
	templates   *template.Template
	mux         *http.ServeMux
}

type layoutData struct {
	Title string
	Body  template.HTML
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
		db:          opts.DB,
		replay:      opts.Replay,
		broadcaster: opts.Broadcaster,
		rt:          opts.Runtime,
		templates:   tpls,
		mux:         http.NewServeMux(),
	}
	s.registerRoutes()

	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := s.recoverMiddleware(s.requestLogger(s.onboardingMiddleware(s.mux)))
	handler.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/runs", http.StatusSeeOther)
	})
	s.mux.HandleFunc("GET /api/sessions", s.handleSessionsIndex)
	s.mux.HandleFunc("GET /api/sessions/{id}", s.handleSessionDetail)
	s.mux.HandleFunc("GET /runs", s.handleRunsIndex)
	s.mux.HandleFunc("GET /runs/{id}", s.handleRunDetail)
	s.mux.HandleFunc("GET /runs/{id}/events", s.handleRunEvents)
	s.mux.Handle("POST /runs/{id}/dismiss", s.adminAuth(http.HandlerFunc(s.handleRunDismiss)))
	s.mux.HandleFunc("GET /approvals", s.handleApprovals)
	s.mux.Handle("POST /approvals/{id}/resolve", s.adminAuth(http.HandlerFunc(s.handleApprovalResolve)))
	s.mux.HandleFunc("GET /settings", s.handleSettings)
	s.mux.Handle("POST /settings", s.adminAuth(http.HandlerFunc(s.handleSettingsUpdate)))
	s.mux.HandleFunc("GET /run", s.handleRunForm)
	s.mux.Handle("POST /run", s.adminAuth(http.HandlerFunc(s.handleRunSubmit)))
	s.mux.HandleFunc("GET /onboarding", s.handleOnboarding)
	s.mux.HandleFunc("POST /onboarding", s.handleOnboardingStep1Submit)
	s.mux.HandleFunc("GET /onboarding/step/2", s.handleOnboardingStep2)
	s.mux.HandleFunc("GET /onboarding/step/3", s.handleOnboardingStep3)
	s.mux.HandleFunc("POST /onboarding/step/3", s.handleOnboardingStep3Submit)
	s.mux.HandleFunc("GET /onboarding/step/4/{id}", s.handleOnboardingStep4)
	s.mux.HandleFunc("GET /memory", s.handleMemoryList)
	s.mux.HandleFunc("POST /memory/{id}/forget", s.handleMemoryForget)
	s.mux.HandleFunc("POST /memory/{id}/edit", s.handleMemoryEdit)
}

func (s *Server) renderTemplate(w http.ResponseWriter, title, bodyTemplate string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var body bytes.Buffer
	if err := s.templates.ExecuteTemplate(&body, bodyTemplate, data); err != nil {
		http.Error(w, fmt.Sprintf("template render failed: %v", err), http.StatusInternalServerError)
		return
	}
	if err := s.templates.ExecuteTemplate(w, "layout", layoutData{
		Title: title,
		Body:  template.HTML(body.String()),
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

func (s *Server) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			s.writeUnauthorized(w)
			return
		}

		adminToken := lookupSetting(s.db, "admin_token")
		if adminToken == "" || r.Header.Get("Authorization") != "Bearer "+adminToken {
			s.writeUnauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("unauthorized"))
}

func loadTemplates() (*template.Template, error) {
	_, currentFile, _, ok := goruntime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("web: locate template directory")
	}

	templateDir := filepath.Join(filepath.Dir(currentFile), "templates")
	tpls, err := template.ParseFiles(
		filepath.Join(templateDir, "layout.html"),
		filepath.Join(templateDir, "runs.html"),
		filepath.Join(templateDir, "run_detail.html"),
		filepath.Join(templateDir, "run_submit.html"),
		filepath.Join(templateDir, "approvals.html"),
		filepath.Join(templateDir, "settings.html"),
		filepath.Join(templateDir, "onboarding.html"),
		filepath.Join(templateDir, "memory.html"),
	)
	if err != nil {
		return nil, fmt.Errorf("web: parse templates: %w", err)
	}
	return tpls, nil
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
