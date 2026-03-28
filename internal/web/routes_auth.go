package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

type loginPageData struct {
	Next               string
	PasswordConfigured bool
	SetupRequired      bool
	Message            string
	MessageClass       string
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if result := s.authenticateRequest(r, false); result.Authenticated {
		http.Redirect(w, r, safeRedirectPath(r.URL.Query().Get("next"), pageWork), http.StatusSeeOther)
		return
	}
	s.handleSPADocument(w, r)
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

	issued, err := s.changePasswordAndReauthenticate(
		r,
		r.FormValue("current_password"),
		r.FormValue("new_password"),
		r.FormValue("confirm_password"),
	)
	if err != nil {
		if errors.Is(err, errPasswordReauthenticationFailed) {
			clearAuthCookies(w, r)
			http.Redirect(w, r, pageLogin+"?reason=expired", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, settingsAccessRedirectPath("", passwordChangeErrorMessage(err)), http.StatusSeeOther)
		return
	}

	setAuthCookies(w, r, issued)
	http.Redirect(w, r, settingsAccessRedirectPath("Password updated. Other device sessions were signed out.", ""), http.StatusSeeOther)
}

func (s *Server) handleDeviceRevoke(w http.ResponseWriter, r *http.Request) {
	s.handleDeviceMutation(w, r, func(ctxRequest *http.Request, deviceID string, current bool) (settingsDeviceMutationResult, error) {
		if err := authpkg.RevokeDeviceSessions(ctxRequest.Context(), s.db, deviceID, time.Now().UTC()); err != nil {
			return settingsDeviceMutationResult{}, err
		}
		if current {
			return settingsDeviceMutationResult{LoggedOut: true, Next: pageLogin + "?reason=logged_out"}, nil
		}
		return settingsDeviceMutationResult{Notice: "Device access revoked."}, nil
	})
}

func (s *Server) handleDeviceBlock(w http.ResponseWriter, r *http.Request) {
	s.handleDeviceMutation(w, r, func(ctxRequest *http.Request, deviceID string, current bool) (settingsDeviceMutationResult, error) {
		if err := authpkg.BlockDevice(ctxRequest.Context(), s.db, deviceID, time.Now().UTC()); err != nil {
			return settingsDeviceMutationResult{}, err
		}
		if current {
			return settingsDeviceMutationResult{LoggedOut: true, Next: pageLogin + "?reason=blocked"}, nil
		}
		return settingsDeviceMutationResult{Notice: "Device blocked."}, nil
	})
}

func (s *Server) handleDeviceUnblock(w http.ResponseWriter, r *http.Request) {
	s.handleDeviceMutation(w, r, func(ctxRequest *http.Request, deviceID string, _ bool) (settingsDeviceMutationResult, error) {
		if err := authpkg.UnblockDevice(ctxRequest.Context(), s.db, deviceID, time.Now().UTC()); err != nil {
			return settingsDeviceMutationResult{}, err
		}
		return settingsDeviceMutationResult{Notice: "Device unblocked."}, nil
	})
}

func (s *Server) handleDeviceMutation(w http.ResponseWriter, r *http.Request, mutate func(*http.Request, string, bool) (settingsDeviceMutationResult, error)) {
	result, err := s.mutateSettingsDevice(r, mutate)
	if err != nil {
		if errors.Is(err, errSettingsSessionRequired) {
			s.writeUnauthorized(w)
			return
		}
		if errors.Is(err, errSettingsDeviceMissing) {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		http.Redirect(w, r, settingsAccessRedirectPath("", deviceMutationErrorMessage(err)), http.StatusSeeOther)
		return
	}

	redirectTo := pageConfigureSettings
	if result.Notice != "" {
		redirectTo = settingsAccessRedirectPath(result.Notice, "")
	}
	if result.LoggedOut && result.Next != "" {
		redirectTo = result.Next
		clearAuthCookies(w, r)
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
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
