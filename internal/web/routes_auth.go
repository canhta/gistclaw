package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
)

var (
	errPasswordConfirmationMismatch   = errors.New("web: password confirmation mismatch")
	errPasswordReauthenticationFailed = errors.New("web: password reauthentication failed")
	errSettingsSessionRequired        = errors.New("web: settings session required")
	errSettingsDeviceMissing          = errors.New("web: settings device missing")
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if result := s.authenticateRequest(r, false); result.Authenticated {
		http.Redirect(w, r, safeRedirectPath(r.URL.Query().Get("next"), s.defaultEntryPath()), http.StatusSeeOther)
		return
	}
	s.handleSPADocument(w, r)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		if err := authpkg.LogoutSession(r.Context(), s.db, cookie.Value, time.Now().UTC()); err != nil {
			http.Error(w, "failed to clear session", http.StatusInternalServerError)
			return
		}
	}
	clearAuthCookies(w, r)
	http.Redirect(w, r, pageLogin+"?reason=logged_out", http.StatusSeeOther)
}

func (s *Server) changePasswordAndReauthenticate(r *http.Request, currentPassword, newPassword, confirmPassword string) (authpkg.IssuedSession, error) {
	if strings.TrimSpace(newPassword) == "" {
		return authpkg.IssuedSession{}, authpkg.ErrPasswordRequired
	}
	if newPassword != confirmPassword {
		return authpkg.IssuedSession{}, errPasswordConfirmationMismatch
	}
	if err := authpkg.ChangePassword(r.Context(), s.db, currentPassword, newPassword, time.Now().UTC()); err != nil {
		return authpkg.IssuedSession{}, err
	}

	deviceCookieValue := ""
	if cookie, err := r.Cookie(deviceCookieName); err == nil {
		deviceCookieValue = cookie.Value
	}
	issued, err := authpkg.Login(r.Context(), s.db, authpkg.LoginInput{
		Password:          newPassword,
		DeviceCookieValue: deviceCookieValue,
		UserAgent:         r.UserAgent(),
		IP:                requestIP(r),
		Now:               time.Now().UTC(),
	})
	if err != nil {
		return authpkg.IssuedSession{}, fmt.Errorf("%w: %v", errPasswordReauthenticationFailed, err)
	}
	return issued, nil
}

func (s *Server) mutateSettingsDevice(
	r *http.Request,
	mutate func(*http.Request, string, bool) (settingsDeviceMutationResult, error),
) (settingsDeviceMutationResult, error) {
	session, ok := requestSessionFromContext(r.Context())
	if !ok {
		return settingsDeviceMutationResult{}, errSettingsSessionRequired
	}
	deviceID := strings.TrimSpace(r.PathValue("id"))
	if deviceID == "" {
		return settingsDeviceMutationResult{}, errSettingsDeviceMissing
	}
	return mutate(r, deviceID, session.DeviceID == deviceID)
}

func loginFailureMessage(err error) string {
	switch {
	case errors.Is(err, authpkg.ErrInvalidPassword):
		return "Password did not match. Try again."
	case errors.Is(err, authpkg.ErrPasswordNotSet):
		return ""
	case errors.Is(err, authpkg.ErrDeviceBlocked):
		return "This device has been blocked. Use another authorized device or reset access locally with the CLI."
	default:
		return "Unable to sign in right now. Try again."
	}
}

func passwordChangeErrorMessage(err error) string {
	switch {
	case errors.Is(err, authpkg.ErrInvalidPassword):
		return "Current password did not match."
	case errors.Is(err, authpkg.ErrPasswordRequired):
		return "Enter a new password."
	case errors.Is(err, errPasswordConfirmationMismatch):
		return "New password confirmation did not match."
	default:
		return "Unable to update the password right now."
	}
}

func deviceMutationErrorMessage(err error) string {
	switch {
	case errors.Is(err, authpkg.ErrDeviceNotFound):
		return "That device no longer exists."
	default:
		return "Unable to update device access right now."
	}
}

func safeRedirectPath(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return fallback
	}
	if !isCurrentAppPath(raw) {
		return fallback
	}
	return raw
}

func isCurrentAppPath(path string) bool {
	switch {
	case path == "/":
		return true
	case path == pageLogin, strings.HasPrefix(path, pageLogin+"?"):
		return true
	case path == pageOnboarding, strings.HasPrefix(path, pageOnboarding+"/"), strings.HasPrefix(path, pageOnboarding+"?"):
		return true
	case path == pageChat, strings.HasPrefix(path, pageChat+"/"), strings.HasPrefix(path, pageChat+"?"):
		return true
	case path == pageChannels, strings.HasPrefix(path, pageChannels+"?"):
		return true
	case path == pageInstances, strings.HasPrefix(path, pageInstances+"?"):
		return true
	case path == pageSessions, strings.HasPrefix(path, pageSessions+"/"), strings.HasPrefix(path, pageSessions+"?"):
		return true
	case path == pageCron, strings.HasPrefix(path, pageCron+"?"):
		return true
	case path == pageSkills, strings.HasPrefix(path, pageSkills+"?"):
		return true
	case path == pageNodes, strings.HasPrefix(path, pageNodes+"?"):
		return true
	case path == pageApprovals, strings.HasPrefix(path, pageApprovals+"?"):
		return true
	case path == pageConfig, strings.HasPrefix(path, pageConfig+"?"):
		return true
	case path == pageDebug, strings.HasPrefix(path, pageDebug+"?"):
		return true
	case path == pageLogs, strings.HasPrefix(path, pageLogs+"?"):
		return true
	case path == pageUpdate, strings.HasPrefix(path, pageUpdate+"?"):
		return true
	default:
		return false
	}
}
