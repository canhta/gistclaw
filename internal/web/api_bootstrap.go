package web

import (
	"net/http"

	authpkg "github.com/canhta/gistclaw/internal/auth"
	"github.com/canhta/gistclaw/internal/runtime"
)

type bootstrapResponse struct {
	Auth       authSessionResponse      `json:"auth"`
	Project    bootstrapProjectResponse `json:"project"`
	Navigation []bootstrapNavItem       `json:"navigation"`
}

type bootstrapProjectResponse struct {
	ActiveID   string `json:"active_id"`
	ActiveName string `json:"active_name"`
	ActivePath string `json:"active_path"`
}

type bootstrapNavItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Href  string `json:"href"`
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	project, err := runtime.ActiveProject(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load active project", http.StatusInternalServerError)
		return
	}
	configured, err := authpkg.PasswordConfigured(r.Context(), s.db)
	if err != nil {
		http.Error(w, "failed to load auth state", http.StatusInternalServerError)
		return
	}
	auth, _ := requestAuthFromContext(r.Context())

	writeJSON(w, http.StatusOK, bootstrapResponse{
		Auth: authSessionResponse{
			Authenticated:      auth.Bearer || auth.Session.SessionID != "",
			PasswordConfigured: configured,
			SetupRequired:      !configured,
			DeviceID:           auth.Session.DeviceID,
		},
		Project: bootstrapProjectResponse{
			ActiveID:   project.ID,
			ActiveName: project.Name,
			ActivePath: project.PrimaryPath,
		},
		Navigation: []bootstrapNavItem{
			{ID: "work", Label: "Work", Href: pageWork},
			{ID: "team", Label: "Team", Href: pageTeam},
			{ID: "knowledge", Label: "Knowledge", Href: pageKnowledge},
			{ID: "recover", Label: "Recover", Href: pageRecover},
			{ID: "conversations", Label: "Conversations", Href: pageConversations},
			{ID: "automate", Label: "Automate", Href: pageAutomate},
			{ID: "history", Label: "History", Href: pageHistory},
			{ID: "settings", Label: "Settings", Href: pageSettings},
		},
	})
}
