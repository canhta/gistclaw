package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
)

type authSessionResponse struct {
	Authenticated      bool   `json:"authenticated"`
	PasswordConfigured bool   `json:"password_configured"`
	SetupRequired      bool   `json:"setup_required"`
	LoginReason        string `json:"login_reason,omitempty"`
	DeviceID           string `json:"device_id,omitempty"`
}

type authLoginRequest struct {
	Password string `json:"password"`
	Next     string `json:"next,omitempty"`
}

type authLoginResponse struct {
	Authenticated bool   `json:"authenticated"`
	Next          string `json:"next"`
}

type authLogoutResponse struct {
	LoggedOut bool `json:"logged_out"`
}

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	configured, err := authpkg.PasswordConfigured(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load auth state", http.StatusInternalServerError)
		return
	}

	result := s.authenticateRequest(r, false)
	resp := authSessionResponse{
		Authenticated:      result.Authenticated,
		PasswordConfigured: configured,
		SetupRequired:      !configured,
		LoginReason:        result.LoginReason,
	}
	if result.Authenticated {
		resp.DeviceID = strings.TrimSpace(result.RequestAuth.Session.DeviceID)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req authLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	deviceCookieValue := ""
	if cookie, err := r.Cookie(deviceCookieName); err == nil {
		deviceCookieValue = cookie.Value
	}

	issued, err := authpkg.Login(r.Context(), s.db, authpkg.LoginInput{
		Password:          strings.TrimSpace(req.Password),
		DeviceCookieValue: deviceCookieValue,
		UserAgent:         r.UserAgent(),
		IP:                requestIP(r),
		Now:               time.Now().UTC(),
	})
	if err != nil {
		message := loginFailureMessage(err)
		if strings.TrimSpace(message) == "" {
			message = "Unable to sign in right now."
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": message})
		return
	}

	setAuthCookies(w, r, issued)
	writeJSON(w, http.StatusOK, authLoginResponse{
		Authenticated: true,
		Next:          safeRedirectPath(req.Next, pageWork),
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		if err := authpkg.LogoutSession(r.Context(), s.db, cookie.Value, time.Now().UTC()); err != nil {
			http.Error(w, "failed to clear session", http.StatusInternalServerError)
			return
		}
	}
	clearAuthCookies(w, r)
	writeJSON(w, http.StatusOK, authLogoutResponse{LoggedOut: true})
}
