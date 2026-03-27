package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	authpkg "github.com/canhta/gistclaw/internal/auth"
)

type loginPageData struct {
	Next               string
	PasswordConfigured bool
	SetupRequired      bool
	Message            string
	MessageClass       string
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if result := s.authenticateRequest(r, false); result.Authenticated {
		http.Redirect(w, r, safeRedirectPath(r.URL.Query().Get("next"), pageOperateRuns), http.StatusSeeOther)
		return
	}

	data, status, err := s.loginPageData(r, "")
	if err != nil {
		http.Error(w, "failed to load login state", http.StatusInternalServerError)
		return
	}
	s.renderAuthTemplateStatus(w, r, status, "Login", "login_body", data)
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	next := safeRedirectPath(r.FormValue("next"), pageOperateRuns)
	password := r.FormValue("password")
	deviceCookieValue := ""
	if cookie, err := r.Cookie(deviceCookieName); err == nil {
		deviceCookieValue = cookie.Value
	}

	issued, err := authpkg.Login(r.Context(), s.db, authpkg.LoginInput{
		Password:          password,
		DeviceCookieValue: deviceCookieValue,
		UserAgent:         r.UserAgent(),
		IP:                requestIP(r),
		Now:               time.Now().UTC(),
	})
	if err != nil {
		data, status, dataErr := s.loginPageData(r, loginFailureMessage(err))
		if dataErr != nil {
			http.Error(w, "failed to load login state", http.StatusInternalServerError)
			return
		}
		s.renderAuthTemplateStatus(w, r, status, "Login", "login_body", data)
		return
	}

	setAuthCookies(w, r, issued)
	http.Redirect(w, r, next, http.StatusSeeOther)
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

func (s *Server) handleSettingsPasswordChange(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")
	if strings.TrimSpace(newPassword) == "" {
		http.Redirect(w, r, settingsAccessRedirectPath("", "Enter a new password."), http.StatusSeeOther)
		return
	}
	if newPassword != confirmPassword {
		http.Redirect(w, r, settingsAccessRedirectPath("", "New password confirmation did not match."), http.StatusSeeOther)
		return
	}

	if err := authpkg.ChangePassword(r.Context(), s.db, currentPassword, newPassword, time.Now().UTC()); err != nil {
		http.Redirect(w, r, settingsAccessRedirectPath("", passwordChangeErrorMessage(err)), http.StatusSeeOther)
		return
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
		clearAuthCookies(w, r)
		http.Redirect(w, r, pageLogin+"?reason=expired", http.StatusSeeOther)
		return
	}

	setAuthCookies(w, r, issued)
	http.Redirect(w, r, settingsAccessRedirectPath("Password updated. Other device sessions were signed out.", ""), http.StatusSeeOther)
}

func (s *Server) handleDeviceRevoke(w http.ResponseWriter, r *http.Request) {
	s.handleDeviceMutation(w, r, func(ctxRequest *http.Request, deviceID string, session authpkg.SessionState) (string, string, error) {
		if err := authpkg.RevokeDeviceSessions(ctxRequest.Context(), s.db, deviceID, time.Now().UTC()); err != nil {
			return "", "", err
		}
		if session.DeviceID == deviceID {
			return pageLogin + "?reason=logged_out", "", nil
		}
		return settingsAccessRedirectPath("Device access revoked.", ""), "", nil
	})
}

func (s *Server) handleDeviceBlock(w http.ResponseWriter, r *http.Request) {
	s.handleDeviceMutation(w, r, func(ctxRequest *http.Request, deviceID string, session authpkg.SessionState) (string, string, error) {
		if err := authpkg.BlockDevice(ctxRequest.Context(), s.db, deviceID, time.Now().UTC()); err != nil {
			return "", "", err
		}
		if session.DeviceID == deviceID {
			return pageLogin + "?reason=blocked", "", nil
		}
		return settingsAccessRedirectPath("Device blocked.", ""), "", nil
	})
}

func (s *Server) handleDeviceUnblock(w http.ResponseWriter, r *http.Request) {
	s.handleDeviceMutation(w, r, func(ctxRequest *http.Request, deviceID string, session authpkg.SessionState) (string, string, error) {
		if err := authpkg.UnblockDevice(ctxRequest.Context(), s.db, deviceID, time.Now().UTC()); err != nil {
			return "", "", err
		}
		return settingsAccessRedirectPath("Device unblocked.", ""), "", nil
	})
}

func (s *Server) handleDeviceMutation(w http.ResponseWriter, r *http.Request, mutate func(*http.Request, string, authpkg.SessionState) (string, string, error)) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	session, ok := requestSessionFromContext(r.Context())
	if !ok {
		s.writeUnauthorized(w)
		return
	}
	deviceID := strings.TrimSpace(r.PathValue("id"))
	if deviceID == "" {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}

	redirectTo, notice, err := mutate(r, deviceID, session)
	if err != nil {
		http.Redirect(w, r, settingsAccessRedirectPath("", deviceMutationErrorMessage(err)), http.StatusSeeOther)
		return
	}
	if notice != "" {
		redirectTo = settingsAccessRedirectPath(notice, "")
	}
	if redirectTo == "" {
		redirectTo = pageConfigureSettings
	}
	if session.DeviceID == deviceID {
		clearAuthCookies(w, r)
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (s *Server) loginPageData(r *http.Request, explicitMessage string) (loginPageData, int, error) {
	configured, err := authpkg.PasswordConfigured(r.Context(), s.db)
	if err != nil {
		return loginPageData{}, 0, err
	}

	data := loginPageData{
		Next:               safeRedirectPath(r.FormValue("next"), safeRedirectPath(r.URL.Query().Get("next"), pageOperateRuns)),
		PasswordConfigured: configured,
	}
	if !configured {
		data.SetupRequired = true
		return data, http.StatusOK, nil
	}

	if explicitMessage != "" {
		data.Message = explicitMessage
		data.MessageClass = "error"
		return data, http.StatusOK, nil
	}

	switch r.URL.Query().Get("reason") {
	case "expired":
		data.Message = "Your session expired. Sign in again to continue."
		data.MessageClass = "notice-panel"
	case "logged_out":
		data.Message = "You signed out of this browser."
		data.MessageClass = "notice-panel"
	case "blocked":
		data.Message = "This device has been blocked. Use another authorized device or reset access locally with the CLI."
		data.MessageClass = "error"
	}
	return data, http.StatusOK, nil
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

func settingsAccessRedirectPath(notice, accessError string) string {
	values := url.Values{}
	if strings.TrimSpace(notice) != "" {
		values.Set("access_notice", notice)
	}
	if strings.TrimSpace(accessError) != "" {
		values.Set("access_error", accessError)
	}
	if len(values) == 0 {
		return pageConfigureSettings
	}
	return pageConfigureSettings + "?" + values.Encode()
}

func safeRedirectPath(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return fallback
	}
	return raw
}
