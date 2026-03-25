package web

import (
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/teams"
)

type teamPageData struct {
	Name       string
	FrontAgent string
	Agents     []teamAgentCardData
	Error      string
}

type teamAgentCardData struct {
	ID          string
	Role        string
	ToolPosture string
	CanSpawn    string
	CanMessage  string
	IsFront     bool
}

func (s *Server) handleTeam(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	cfg, err := s.rt.TeamConfig()
	if err != nil {
		s.renderTemplate(w, "Team", "team_body", teamPageData{Error: err.Error()})
		return
	}

	s.renderTemplate(w, "Team", "team_body", newTeamPageData(cfg, ""))
}

func (s *Server) handleTeamUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	cfg, err := s.rt.TeamConfig()
	if err != nil {
		s.renderTemplate(w, "Team", "team_body", teamPageData{Error: err.Error()})
		return
	}

	cfg.Name = strings.TrimSpace(r.FormValue("name"))
	cfg.FrontAgent = strings.TrimSpace(r.FormValue("front_agent"))
	for i := range cfg.Agents {
		agent := &cfg.Agents[i]
		prefix := "agent_" + agent.ID + "_"
		agent.Role = strings.TrimSpace(r.FormValue(prefix + "role"))
		agent.ToolPosture = strings.TrimSpace(r.FormValue(prefix + "tool_posture"))
		agent.CanSpawn = parseAgentLinks(r.FormValue(prefix + "can_spawn"))
		agent.CanMessage = parseAgentLinks(r.FormValue(prefix + "can_message"))
	}

	if cfg.Name == "" {
		s.renderTemplateStatus(w, http.StatusUnprocessableEntity, "Team", "team_body", newTeamPageData(cfg, "team name is required"))
		return
	}
	if cfg.FrontAgent == "" {
		s.renderTemplateStatus(w, http.StatusUnprocessableEntity, "Team", "team_body", newTeamPageData(cfg, "front agent is required"))
		return
	}

	if err := s.rt.UpdateTeam(r.Context(), cfg); err != nil {
		s.renderTemplateStatus(w, http.StatusUnprocessableEntity, "Team", "team_body", newTeamPageData(cfg, err.Error()))
		return
	}

	http.Redirect(w, r, "/team", http.StatusSeeOther)
}

func newTeamPageData(cfg teams.Config, errMsg string) teamPageData {
	data := teamPageData{
		Name:       cfg.Name,
		FrontAgent: cfg.FrontAgent,
		Agents:     make([]teamAgentCardData, 0, len(cfg.Agents)),
		Error:      errMsg,
	}
	for _, agent := range cfg.Agents {
		data.Agents = append(data.Agents, teamAgentCardData{
			ID:          agent.ID,
			Role:        agent.Role,
			ToolPosture: agent.ToolPosture,
			CanSpawn:    joinAgentLinks(agent.CanSpawn),
			CanMessage:  joinAgentLinks(agent.CanMessage),
			IsFront:     agent.ID == cfg.FrontAgent,
		})
	}
	return data
}

func parseAgentLinks(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]bool, len(parts))
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		items = append(items, value)
	}
	return items
}

func joinAgentLinks(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, ", ")
}
