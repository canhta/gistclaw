package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/canhta/gistclaw/internal/teams"
)

const teamUploadLimit = 1 << 20

type teamPageData struct {
	Name       string
	FrontAgent string
	Agents     []teamAgentCardData
	Error      string
	Notice     string
}

type teamAgentCardData struct {
	Index              int
	ID                 string
	SoulFile           string
	SoulExtraJSON      string
	Role               string
	ToolPosture        string
	ToolPostureOptions []teamOption
	CanSpawnOptions    []teamOption
	CanMessageOptions  []teamOption
	IsFront            bool
}

type teamOption struct {
	Value    string
	Label    string
	Selected bool
}

func (s *Server) handleTeam(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadTeamConfig()
	if err != nil {
		s.renderTemplate(w, "Team", "team_body", teamPageData{Error: err.Error()})
		return
	}

	s.renderTemplate(w, "Team", "team_body", newTeamPageData(cfg, "", ""))
}

func (s *Server) handleTeamExport(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadTeamConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	raw, err := teams.ExportEditableYAML(cfg)
	if err != nil {
		http.Error(w, "failed to export team file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="team.yaml"`)
	_, _ = w.Write(raw)
}

func (s *Server) handleTeamUpdate(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(teamUploadLimit); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
	}

	intent := strings.TrimSpace(r.FormValue("intent"))
	if intent == "" {
		intent = "save"
	}
	removeIndex := strings.TrimSpace(r.FormValue("remove_agent_index"))
	if strings.HasPrefix(intent, "remove_member:") {
		removeIndex = strings.TrimPrefix(intent, "remove_member:")
		intent = "remove_member"
	}

	switch intent {
	case "import":
		cfg, err := parseImportedTeamConfig(r)
		if err != nil {
			s.renderStoredTeamError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.renderTemplate(w, "Team", "team_body", newTeamPageData(cfg, "", "Imported file loaded. Save Team to apply the change."))
		return
	case "add_member", "remove_member", "save":
	default:
		intent = "save"
	}

	cfg, err := teamConfigFromRequest(r)
	if err != nil {
		s.renderStoredTeamError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	switch intent {
	case "add_member":
		cfg = addTeamMember(cfg)
		s.renderTemplate(w, "Team", "team_body", newTeamPageData(cfg, "", "New member added. Save Team to apply the change."))
		return
	case "remove_member":
		index, err := strconv.Atoi(removeIndex)
		if err != nil {
			s.renderTemplateStatus(w, http.StatusUnprocessableEntity, "Team", "team_body", newTeamPageData(cfg, "invalid member removal request", ""))
			return
		}
		if err := removeTeamMember(&cfg, index); err != nil {
			s.renderTemplateStatus(w, http.StatusUnprocessableEntity, "Team", "team_body", newTeamPageData(cfg, err.Error(), ""))
			return
		}
		s.renderTemplate(w, "Team", "team_body", newTeamPageData(cfg, "", "Member removed. Save Team to apply the change."))
		return
	default:
		if s.rt == nil {
			http.Error(w, "runtime not configured", http.StatusInternalServerError)
			return
		}
		if err := s.rt.UpdateTeam(r.Context(), cfg); err != nil {
			s.renderTemplateStatus(w, http.StatusUnprocessableEntity, "Team", "team_body", newTeamPageData(cfg, err.Error(), ""))
			return
		}
		http.Redirect(w, r, "/team", http.StatusSeeOther)
	}
}

func (s *Server) loadTeamConfig() (teams.Config, error) {
	if s.rt == nil {
		return teams.Config{}, fmt.Errorf("runtime: team dir not configured")
	}
	cfg, err := s.rt.TeamConfig()
	if err != nil {
		return teams.Config{}, err
	}
	return cfg, nil
}

func (s *Server) renderStoredTeamError(w http.ResponseWriter, status int, errMsg string) {
	cfg, err := s.loadTeamConfig()
	if err != nil {
		s.renderTemplateStatus(w, status, "Team", "team_body", teamPageData{Error: errMsg})
		return
	}
	s.renderTemplateStatus(w, status, "Team", "team_body", newTeamPageData(cfg, errMsg, ""))
}

func newTeamPageData(cfg teams.Config, errMsg, notice string) teamPageData {
	data := teamPageData{
		Name:       cfg.Name,
		FrontAgent: cfg.FrontAgent,
		Agents:     make([]teamAgentCardData, 0, len(cfg.Agents)),
		Error:      errMsg,
		Notice:     notice,
	}
	for idx, agent := range cfg.Agents {
		soulExtraJSON := "{}"
		if len(agent.Soul.Extra) > 0 {
			if raw, err := json.Marshal(agent.Soul.Extra); err == nil {
				soulExtraJSON = string(raw)
			}
		}
		data.Agents = append(data.Agents, teamAgentCardData{
			Index:              idx,
			ID:                 agent.ID,
			SoulFile:           agent.SoulFile,
			SoulExtraJSON:      soulExtraJSON,
			Role:               agent.Role,
			ToolPosture:        agent.ToolPosture,
			ToolPostureOptions: buildToolPostureOptions(agent.ToolPosture),
			CanSpawnOptions:    buildTeamLinkOptions(cfg.Agents, idx, agent.CanSpawn),
			CanMessageOptions:  buildTeamLinkOptions(cfg.Agents, idx, agent.CanMessage),
			IsFront:            agent.ID == cfg.FrontAgent,
		})
	}
	return data
}

func teamConfigFromRequest(r *http.Request) (teams.Config, error) {
	count, err := strconv.Atoi(strings.TrimSpace(r.FormValue("agent_count")))
	if err != nil || count < 0 {
		return teams.Config{}, fmt.Errorf("team editor is missing agent state")
	}

	cfg := teams.Config{
		Name:       strings.TrimSpace(r.FormValue("name")),
		FrontAgent: strings.TrimSpace(r.FormValue("front_agent")),
		Agents:     make([]teams.AgentConfig, 0, count),
	}
	for idx := 0; idx < count; idx++ {
		prefix := fmt.Sprintf("agent_%d_", idx)
		agentID := strings.TrimSpace(r.FormValue(prefix + "id"))
		soulFile := strings.TrimSpace(r.FormValue(prefix + "soul_file"))
		if soulFile == "" {
			soulFile = teams.SuggestedSoulFile(agentID)
		}
		role := strings.TrimSpace(r.FormValue(prefix + "role"))
		toolPosture := strings.TrimSpace(r.FormValue(prefix + "tool_posture"))
		agent := teams.AgentConfig{
			ID:          agentID,
			SoulFile:    soulFile,
			Role:        role,
			ToolPosture: toolPosture,
			CanSpawn:    normalizeAgentLinks(r.Form[prefix+"can_spawn"]),
			CanMessage:  normalizeAgentLinks(r.Form[prefix+"can_message"]),
			Soul: teams.SoulSpec{
				Role:        role,
				ToolPosture: toolPosture,
				Extra:       parseSoulExtraJSON(strings.TrimSpace(r.FormValue(prefix + "soul_extra_json"))),
			},
		}
		cfg.Agents = append(cfg.Agents, agent)
	}
	return cfg, nil
}

func parseImportedTeamConfig(r *http.Request) (teams.Config, error) {
	file, _, err := r.FormFile("import_file")
	if err != nil {
		return teams.Config{}, fmt.Errorf("team import file is required")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, teamUploadLimit))
	if err != nil {
		return teams.Config{}, fmt.Errorf("failed to read import file: %w", err)
	}
	cfg, err := teams.LoadEditableYAML(data)
	if err != nil {
		return teams.Config{}, err
	}
	return cfg, nil
}

func parseSoulExtraJSON(raw string) map[string]any {
	if raw == "" {
		return map[string]any{}
	}
	var extra map[string]any
	if err := json.Unmarshal([]byte(raw), &extra); err != nil {
		return map[string]any{}
	}
	if extra == nil {
		return map[string]any{}
	}
	return extra
}

func normalizeAgentLinks(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		items = append(items, value)
	}
	return items
}

func addTeamMember(cfg teams.Config) teams.Config {
	id := nextTeamAgentID(cfg)
	cfg.Agents = append(cfg.Agents, teams.AgentConfig{
		ID:          id,
		SoulFile:    teams.SuggestedSoulFile(id),
		Role:        "new specialist",
		ToolPosture: "read_heavy",
		Soul: teams.SoulSpec{
			Role:        "new specialist",
			ToolPosture: "read_heavy",
			Extra:       map[string]any{},
		},
	})
	return cfg
}

func removeTeamMember(cfg *teams.Config, index int) error {
	if index < 0 || index >= len(cfg.Agents) {
		return fmt.Errorf("invalid member removal request")
	}
	if len(cfg.Agents) == 1 {
		return fmt.Errorf("team must keep at least one agent")
	}
	removedID := cfg.Agents[index].ID
	if removedID == cfg.FrontAgent {
		return fmt.Errorf("Choose another front agent before removing %s.", removedID)
	}
	cfg.Agents = append(cfg.Agents[:index], cfg.Agents[index+1:]...)
	for i := range cfg.Agents {
		cfg.Agents[i].CanSpawn = removeAgentLink(cfg.Agents[i].CanSpawn, removedID)
		cfg.Agents[i].CanMessage = removeAgentLink(cfg.Agents[i].CanMessage, removedID)
	}
	return nil
}

func removeAgentLink(values []string, removedID string) []string {
	filtered := values[:0]
	for _, value := range values {
		if value == removedID {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}

func nextTeamAgentID(cfg teams.Config) string {
	seen := make(map[string]bool, len(cfg.Agents))
	for _, agent := range cfg.Agents {
		seen[agent.ID] = true
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("agent_%d", i)
		if !seen[candidate] {
			return candidate
		}
	}
}

func buildTeamLinkOptions(agents []teams.AgentConfig, currentIndex int, selected []string) []teamOption {
	selectedSet := make(map[string]bool, len(selected))
	for _, value := range selected {
		selectedSet[value] = true
	}
	options := make([]teamOption, 0, len(agents))
	for idx, agent := range agents {
		if idx == currentIndex {
			continue
		}
		options = append(options, teamOption{
			Value:    agent.ID,
			Label:    agent.ID,
			Selected: selectedSet[agent.ID],
		})
	}
	return options
}

func buildToolPostureOptions(selected string) []teamOption {
	values := []string{
		"operator_facing",
		"workspace_write",
		"read_heavy",
		"propose_only",
	}
	options := make([]teamOption, 0, len(values))
	for _, value := range values {
		options = append(options, teamOption{
			Value:    value,
			Label:    strings.ReplaceAll(value, "_", " "),
			Selected: value == selected,
		})
	}
	return options
}
