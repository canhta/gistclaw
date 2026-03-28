package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/teams"
)

const teamUploadLimit = 1 << 20

type teamPageData struct {
	ActiveProfile        string
	ProfileOptions       []teamOption
	CloneSourceOptions   []teamOption
	DeleteProfileOptions []teamOption
	ProfileSavePath      string
	Name                 string
	FrontAgent           string
	Agents               []teamAgentCardData
	Error                string
	Notice               string
}

type teamAgentCardData struct {
	Index                    int
	ID                       string
	SoulFile                 string
	SoulExtraJSON            string
	Role                     string
	BaseProfile              string
	BaseProfileOptions       []teamOption
	ToolFamilyOptions        []teamOption
	DelegationKindOptions    []teamOption
	CanMessageOptions        []teamOption
	SpecialistSummaryOptions []teamOption
	IsFront                  bool
}

type teamOption struct {
	Value    string
	Label    string
	Selected bool
}

type teamPageState struct {
	Config          teams.Config
	ActiveProfile   string
	Profiles        []teams.Profile
	ProfileSavePath string
}

func (s *Server) handleTeam(w http.ResponseWriter, r *http.Request) {
	state, err := s.loadTeamPageState(r.Context())
	if err != nil {
		s.renderTemplate(w, r, "Team", "team_body", teamPageData{Error: err.Error()})
		return
	}

	s.renderTemplate(w, r, "Team", "team_body", newTeamPageData(state, "", strings.TrimSpace(r.URL.Query().Get("notice"))))
}

func (s *Server) handleTeamExport(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadTeamConfig(r.Context())
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
	case "select_profile":
		if s.rt == nil {
			http.Error(w, "runtime not configured", http.StatusInternalServerError)
			return
		}
		profile := strings.TrimSpace(r.FormValue("active_profile"))
		if err := s.rt.SelectTeamProfile(r.Context(), profile); err != nil {
			s.renderStoredTeamError(w, r, http.StatusUnprocessableEntity, err.Error())
			return
		}
		http.Redirect(w, r, teamNoticePath(fmt.Sprintf("Active profile switched to %s.", profile)), http.StatusSeeOther)
		return
	case "create_profile":
		if s.rt == nil {
			http.Error(w, "runtime not configured", http.StatusInternalServerError)
			return
		}
		profile := strings.TrimSpace(r.FormValue("create_profile_name"))
		if err := s.rt.CreateTeamProfile(r.Context(), profile); err != nil {
			s.renderStoredTeamError(w, r, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if err := s.rt.SelectTeamProfile(r.Context(), profile); err != nil {
			s.renderStoredTeamError(w, r, http.StatusUnprocessableEntity, err.Error())
			return
		}
		http.Redirect(w, r, teamNoticePath(fmt.Sprintf("Profile %s created and selected.", profile)), http.StatusSeeOther)
		return
	case "clone_profile":
		if s.rt == nil {
			http.Error(w, "runtime not configured", http.StatusInternalServerError)
			return
		}
		sourceProfile := strings.TrimSpace(r.FormValue("clone_source_profile"))
		profile := strings.TrimSpace(r.FormValue("clone_profile_name"))
		if err := s.rt.CloneTeamProfile(r.Context(), sourceProfile, profile); err != nil {
			s.renderStoredTeamError(w, r, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if err := s.rt.SelectTeamProfile(r.Context(), profile); err != nil {
			s.renderStoredTeamError(w, r, http.StatusUnprocessableEntity, err.Error())
			return
		}
		http.Redirect(w, r, teamNoticePath(fmt.Sprintf("Profile %s cloned from %s.", profile, sourceProfile)), http.StatusSeeOther)
		return
	case "delete_profile":
		if s.rt == nil {
			http.Error(w, "runtime not configured", http.StatusInternalServerError)
			return
		}
		profile := strings.TrimSpace(r.FormValue("delete_profile_name"))
		if err := s.rt.DeleteTeamProfile(r.Context(), profile); err != nil {
			s.renderStoredTeamError(w, r, http.StatusUnprocessableEntity, err.Error())
			return
		}
		http.Redirect(w, r, teamNoticePath(fmt.Sprintf("Profile %s deleted.", profile)), http.StatusSeeOther)
		return
	case "import":
		cfg, err := parseImportedTeamConfig(r)
		if err != nil {
			s.renderStoredTeamError(w, r, http.StatusUnprocessableEntity, err.Error())
			return
		}
		state, loadErr := s.loadTeamPageState(r.Context())
		if loadErr != nil {
			s.renderTemplate(w, r, "Team", "team_body", teamPageData{Error: loadErr.Error()})
			return
		}
		state.Config = cfg
		s.renderTemplate(w, r, "Team", "team_body", newTeamPageData(state, "", "Imported file loaded. Save Team to apply the change."))
		return
	case "add_member", "remove_member", "save":
	default:
		intent = "save"
	}

	cfg, err := teamConfigFromRequest(r)
	if err != nil {
		s.renderStoredTeamError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	}

	switch intent {
	case "add_member":
		cfg = addTeamMember(cfg)
		state, loadErr := s.loadTeamPageState(r.Context())
		if loadErr != nil {
			s.renderTemplate(w, r, "Team", "team_body", teamPageData{Error: loadErr.Error()})
			return
		}
		state.Config = cfg
		s.renderTemplate(w, r, "Team", "team_body", newTeamPageData(state, "", "New member added. Save Team to apply the change."))
		return
	case "remove_member":
		index, err := strconv.Atoi(removeIndex)
		if err != nil {
			state, loadErr := s.loadTeamPageState(r.Context())
			if loadErr != nil {
				s.renderTemplateStatus(w, r, http.StatusUnprocessableEntity, "Team", "team_body", teamPageData{Error: "invalid member removal request"})
				return
			}
			state.Config = cfg
			s.renderTemplateStatus(w, r, http.StatusUnprocessableEntity, "Team", "team_body", newTeamPageData(state, "invalid member removal request", ""))
			return
		}
		if err := removeTeamMember(&cfg, index); err != nil {
			state, loadErr := s.loadTeamPageState(r.Context())
			if loadErr != nil {
				s.renderTemplateStatus(w, r, http.StatusUnprocessableEntity, "Team", "team_body", teamPageData{Error: err.Error()})
				return
			}
			state.Config = cfg
			s.renderTemplateStatus(w, r, http.StatusUnprocessableEntity, "Team", "team_body", newTeamPageData(state, err.Error(), ""))
			return
		}
		state, loadErr := s.loadTeamPageState(r.Context())
		if loadErr != nil {
			s.renderTemplate(w, r, "Team", "team_body", teamPageData{Error: loadErr.Error()})
			return
		}
		state.Config = cfg
		s.renderTemplate(w, r, "Team", "team_body", newTeamPageData(state, "", "Member removed. Save Team to apply the change."))
		return
	default:
		if s.rt == nil {
			http.Error(w, "runtime not configured", http.StatusInternalServerError)
			return
		}
		if err := s.rt.UpdateTeam(r.Context(), cfg); err != nil {
			state, loadErr := s.loadTeamPageState(r.Context())
			if loadErr != nil {
				s.renderTemplateStatus(w, r, http.StatusUnprocessableEntity, "Team", "team_body", teamPageData{Error: err.Error()})
				return
			}
			state.Config = cfg
			s.renderTemplateStatus(w, r, http.StatusUnprocessableEntity, "Team", "team_body", newTeamPageData(state, err.Error(), ""))
			return
		}
		http.Redirect(w, r, pageConfigureTeam, http.StatusSeeOther)
	}
}

func (s *Server) loadTeamPageState(ctx context.Context) (teamPageState, error) {
	if s.rt == nil {
		return teamPageState{}, fmt.Errorf("runtime: team dir not configured")
	}
	cfg, err := s.rt.TeamConfig(ctx)
	if err != nil {
		return teamPageState{}, err
	}
	activeProfile, err := s.rt.ActiveTeamProfile(ctx)
	if err != nil {
		return teamPageState{}, err
	}
	profiles, err := s.rt.ListTeamProfiles(ctx)
	if err != nil {
		return teamPageState{}, err
	}
	savePath, err := s.rt.TeamConfigPath(ctx)
	if err != nil {
		return teamPageState{}, err
	}
	return teamPageState{
		Config:          cfg,
		ActiveProfile:   activeProfile,
		Profiles:        profiles,
		ProfileSavePath: savePath,
	}, nil
}

func (s *Server) loadTeamConfig(ctx context.Context) (teams.Config, error) {
	state, err := s.loadTeamPageState(ctx)
	if err != nil {
		return teams.Config{}, err
	}
	return state.Config, nil
}

func (s *Server) renderStoredTeamError(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	state, err := s.loadTeamPageState(r.Context())
	if err != nil {
		s.renderTemplateStatus(w, r, status, "Team", "team_body", teamPageData{Error: errMsg})
		return
	}
	s.renderTemplateStatus(w, r, status, "Team", "team_body", newTeamPageData(state, errMsg, ""))
}

func newTeamPageData(state teamPageState, errMsg, notice string) teamPageData {
	cfg := state.Config
	activeProfile := state.ActiveProfile
	if activeProfile == "" {
		activeProfile = teams.DefaultProfileName
	}

	data := teamPageData{
		ActiveProfile:        activeProfile,
		ProfileOptions:       buildProfileOptions(state.Profiles, activeProfile, true),
		CloneSourceOptions:   buildProfileOptions(state.Profiles, activeProfile, true),
		DeleteProfileOptions: buildProfileOptions(state.Profiles, activeProfile, false),
		ProfileSavePath:      state.ProfileSavePath,
		Name:                 cfg.Name,
		FrontAgent:           cfg.FrontAgent,
		Agents:               make([]teamAgentCardData, 0, len(cfg.Agents)),
		Error:                errMsg,
		Notice:               notice,
	}
	for idx, agent := range cfg.Agents {
		soulExtraJSON := "{}"
		if len(agent.Soul.Extra) > 0 {
			if raw, err := json.Marshal(agent.Soul.Extra); err == nil {
				soulExtraJSON = string(raw)
			}
		}
		data.Agents = append(data.Agents, teamAgentCardData{
			Index:                    idx,
			ID:                       agent.ID,
			SoulFile:                 agent.SoulFile,
			SoulExtraJSON:            soulExtraJSON,
			Role:                     agent.Role,
			BaseProfile:              string(agent.BaseProfile),
			BaseProfileOptions:       buildBaseProfileOptions(agent.BaseProfile),
			ToolFamilyOptions:        buildToolFamilyOptions(agent.ToolFamilies),
			DelegationKindOptions:    buildDelegationKindOptions(agent.DelegationKinds),
			CanMessageOptions:        buildTeamLinkOptions(cfg.Agents, idx, agent.CanMessage),
			SpecialistSummaryOptions: buildSpecialistSummaryOptions(agent.SpecialistSummaryVisibility),
			IsFront:                  agent.ID == cfg.FrontAgent,
		})
	}
	return data
}

func buildProfileOptions(profiles []teams.Profile, activeProfile string, includeActive bool) []teamOption {
	options := make([]teamOption, 0, len(profiles)+1)
	seen := make(map[string]bool, len(profiles)+1)
	if includeActive && activeProfile != "" {
		options = append(options, teamOption{
			Value:    activeProfile,
			Label:    activeProfile,
			Selected: true,
		})
		seen[activeProfile] = true
	}
	for _, profile := range profiles {
		if profile.Name == activeProfile && !includeActive {
			continue
		}
		if seen[profile.Name] {
			continue
		}
		options = append(options, teamOption{
			Value:    profile.Name,
			Label:    profile.Name,
			Selected: profile.Name == activeProfile,
		})
		seen[profile.Name] = true
	}
	return options
}

func teamNoticePath(notice string) string {
	if notice == "" {
		return pageConfigureTeam
	}
	return pageConfigureTeam + "?notice=" + url.QueryEscape(notice)
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
		baseProfile := model.BaseProfile(strings.TrimSpace(r.FormValue(prefix + "base_profile")))
		agent := teams.AgentConfig{
			ID:                          agentID,
			SoulFile:                    soulFile,
			Role:                        role,
			BaseProfile:                 baseProfile,
			ToolFamilies:                normalizeToolFamilies(r.Form[prefix+"tool_families"]),
			DelegationKinds:             normalizeDelegationKinds(r.Form[prefix+"delegation_kinds"]),
			CanMessage:                  normalizeAgentLinks(r.Form[prefix+"can_message"]),
			SpecialistSummaryVisibility: model.SpecialistSummaryVisibility(strings.TrimSpace(r.FormValue(prefix + "specialist_summary_visibility"))),
			Soul: teams.SoulSpec{
				Role:  role,
				Extra: parseSoulExtraJSON(strings.TrimSpace(r.FormValue(prefix + "soul_extra_json"))),
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
		ID:                          id,
		SoulFile:                    teams.SuggestedSoulFile(id),
		Role:                        "research specialist",
		BaseProfile:                 model.BaseProfileResearch,
		ToolFamilies:                []model.ToolFamily{model.ToolFamilyRepoRead, model.ToolFamilyWebRead},
		SpecialistSummaryVisibility: model.SpecialistSummaryBasic,
		Soul: teams.SoulSpec{
			Role:  "research specialist",
			Extra: map[string]any{},
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
		return fmt.Errorf("choose another front agent before removing %s", removedID)
	}
	cfg.Agents = append(cfg.Agents[:index], cfg.Agents[index+1:]...)
	for i := range cfg.Agents {
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

func buildBaseProfileOptions(selected model.BaseProfile) []teamOption {
	values := []model.BaseProfile{
		model.BaseProfileOperator,
		model.BaseProfileResearch,
		model.BaseProfileWrite,
		model.BaseProfileReview,
		model.BaseProfileVerify,
	}
	options := make([]teamOption, 0, len(values))
	for _, value := range values {
		options = append(options, teamOption{
			Value:    string(value),
			Label:    string(value),
			Selected: value == selected,
		})
	}
	return options
}

func buildToolFamilyOptions(selected []model.ToolFamily) []teamOption {
	values := []model.ToolFamily{
		model.ToolFamilyRepoRead,
		model.ToolFamilyRepoWrite,
		model.ToolFamilyRuntimeCapability,
		model.ToolFamilyConnectorCapability,
		model.ToolFamilyWebRead,
		model.ToolFamilyDelegate,
		model.ToolFamilyVerification,
		model.ToolFamilyDiffReview,
	}
	selectedSet := make(map[string]bool, len(selected))
	for _, value := range selected {
		selectedSet[string(value)] = true
	}
	options := make([]teamOption, 0, len(values))
	for _, value := range values {
		options = append(options, teamOption{
			Value:    string(value),
			Label:    strings.ReplaceAll(string(value), "_", " "),
			Selected: selectedSet[string(value)],
		})
	}
	return options
}

func buildDelegationKindOptions(selected []model.DelegationKind) []teamOption {
	values := []model.DelegationKind{
		model.DelegationKindResearch,
		model.DelegationKindWrite,
		model.DelegationKindReview,
		model.DelegationKindVerify,
	}
	selectedSet := make(map[string]bool, len(selected))
	for _, value := range selected {
		selectedSet[string(value)] = true
	}
	options := make([]teamOption, 0, len(values))
	for _, value := range values {
		options = append(options, teamOption{
			Value:    string(value),
			Label:    string(value),
			Selected: selectedSet[string(value)],
		})
	}
	return options
}

func buildSpecialistSummaryOptions(selected model.SpecialistSummaryVisibility) []teamOption {
	values := []model.SpecialistSummaryVisibility{
		model.SpecialistSummaryNone,
		model.SpecialistSummaryBasic,
		model.SpecialistSummaryFull,
	}
	options := make([]teamOption, 0, len(values))
	for _, value := range values {
		options = append(options, teamOption{
			Value:    string(value),
			Label:    string(value),
			Selected: value == selected,
		})
	}
	return options
}

func normalizeToolFamilies(values []string) []model.ToolFamily {
	seen := make(map[string]bool, len(values))
	items := make([]model.ToolFamily, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		items = append(items, model.ToolFamily(value))
	}
	return items
}

func normalizeDelegationKinds(values []string) []model.DelegationKind {
	seen := make(map[string]bool, len(values))
	items := make([]model.DelegationKind, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		items = append(items, model.DelegationKind(value))
	}
	return items
}
