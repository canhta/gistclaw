package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
)

const (
	sessionCookieName = "gistclaw_session"
	deviceCookieName  = "gistclaw_device"
)

type requestAuth struct {
	Bearer  bool
	Session authpkg.SessionState
}

type requestAuthContextKey struct{}

type authResult struct {
	Authenticated      bool
	SameOriginRejected bool
	LoginReason        string
	RequestAuth        requestAuth
}

func (s *Server) authGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		result := s.authenticateRequest(r, false)
		if result.Authenticated {
			next.ServeHTTP(w, withRequestAuth(r, result.RequestAuth))
			return
		}
		if !requiresAuthentication(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if isAPIPath(r.URL.Path) || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			s.writeUnauthorized(w)
			return
		}
		http.Redirect(w, r, loginRedirectPath(r, result.LoginReason), http.StatusSeeOther)
	})
}

func (s *Server) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := s.authenticateRequest(r, true)
		if result.Authenticated {
			next.ServeHTTP(w, withRequestAuth(r, result.RequestAuth))
			return
		}
		if result.SameOriginRejected {
			s.writeForbidden(w)
			return
		}
		s.writeUnauthorized(w)
	})
}

func (s *Server) authenticateRequest(r *http.Request, requireSameOrigin bool) authResult {
	adminToken := lookupSetting(s.db, "admin_token")
	if adminToken != "" && s.authorizedByBearer(r, adminToken) {
		return authResult{
			Authenticated: true,
			RequestAuth: requestAuth{
				Bearer: true,
			},
		}
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return authResult{}
	}
	if requireSameOrigin && !sameOriginRequest(r) {
		return authResult{SameOriginRejected: true}
	}

	state, err := authpkg.AuthenticateSession(r.Context(), s.db, cookie.Value, time.Now().UTC())
	if err == nil {
		return authResult{
			Authenticated: true,
			RequestAuth: requestAuth{
				Session: state,
			},
		}
	}

	switch {
	case errors.Is(err, authpkg.ErrSessionExpired), errors.Is(err, authpkg.ErrSessionRevoked):
		return authResult{LoginReason: "expired"}
	case errors.Is(err, authpkg.ErrDeviceBlocked):
		return authResult{LoginReason: "blocked"}
	default:
		return authResult{}
	}
}

func withRequestAuth(r *http.Request, auth requestAuth) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), requestAuthContextKey{}, auth))
}

func requestAuthFromContext(ctx context.Context) (requestAuth, bool) {
	auth, ok := ctx.Value(requestAuthContextKey{}).(requestAuth)
	return auth, ok
}

func requestSessionFromContext(ctx context.Context) (authpkg.SessionState, bool) {
	auth, ok := requestAuthFromContext(ctx)
	if !ok || auth.Session.SessionID == "" {
		return authpkg.SessionState{}, false
	}
	return auth.Session, true
}

func requestOriginHost(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
		return forwarded
	}
	return r.Host
}

func isPublicPath(path string) bool {
	switch {
	case path == pageLogin, path == pageLogout:
		return true
	case path == "/robots.txt":
		return true
	case path == "/api/auth/session", path == "/api/auth/login":
		return true
	case path == "/_app" || strings.HasPrefix(path, "/_app/"):
		return true
	case strings.HasPrefix(path, "/webhooks/"):
		return true
	default:
		return false
	}
}

func requiresAuthentication(path string) bool {
	switch {
	case path == "/":
		return true
	case path == pageChat, strings.HasPrefix(path, pageChat+"/"):
		return true
	case path == pageChannels, strings.HasPrefix(path, pageChannels+"/"):
		return true
	case path == pageInstances, strings.HasPrefix(path, pageInstances+"/"):
		return true
	case path == pageSessions, strings.HasPrefix(path, pageSessions+"/"):
		return true
	case path == pageCron, strings.HasPrefix(path, pageCron+"/"):
		return true
	case path == pageSkills, strings.HasPrefix(path, pageSkills+"/"):
		return true
	case path == pageNodes, strings.HasPrefix(path, pageNodes+"/"):
		return true
	case path == pageApprovals, strings.HasPrefix(path, pageApprovals+"/"):
		return true
	case path == pageConfig, strings.HasPrefix(path, pageConfig+"/"):
		return true
	case path == pageDebug, strings.HasPrefix(path, pageDebug+"/"):
		return true
	case path == pageLogs, strings.HasPrefix(path, pageLogs+"/"):
		return true
	case path == pageUpdate, strings.HasPrefix(path, pageUpdate+"/"):
		return true
	case path == pageOnboarding, strings.HasPrefix(path, pageOnboarding+"/"):
		return true
	case path == "/api", strings.HasPrefix(path, "/api/"):
		return true
	case path == "/projects/activate":
		return true
	default:
		return false
	}
}

func isAPIPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}

func loginRedirectPath(r *http.Request, reason string) string {
	values := url.Values{}
	if next := safeRedirectPath(requestPathWithQuery(r), ""); next != "" && next != pageLogin {
		values.Set("next", next)
	}
	if strings.TrimSpace(reason) != "" {
		values.Set("reason", reason)
	}
	if len(values) == 0 {
		return pageLogin
	}
	return pageLogin + "?" + values.Encode()
}

func requestIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func setAuthCookies(w http.ResponseWriter, r *http.Request, issued authpkg.IssuedSession) {
	secure := requestIsSecure(r)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    issued.SessionCookieValue,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     deviceCookieName,
		Value:    issued.DeviceCookieValue,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	secure := requestIsSecure(r)
	for _, name := range []string{sessionCookieName, deviceCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   secure,
		})
	}
}
