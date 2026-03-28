package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/teams"
)

type teamAPIResponse struct {
	Notice        string                `json:"notice,omitempty"`
	ActiveProfile teamProfileResponse   `json:"active_profile"`
	Profiles      []teamProfileResponse `json:"profiles"`
	Team          teamConfigResponse    `json:"team"`
}

type teamProfileResponse struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Active   bool   `json:"active"`
	SavePath string `json:"save_path,omitempty"`
}

type teamConfigResponse struct {
	Name         string               `json:"name"`
	FrontAgentID string               `json:"front_agent_id"`
	MemberCount  int                  `json:"member_count"`
	Members      []teamMemberResponse `json:"members"`
}

type teamMemberResponse struct {
	ID                          string         `json:"id"`
	Role                        string         `json:"role"`
	SoulFile                    string         `json:"soul_file"`
	BaseProfile                 string         `json:"base_profile"`
	ToolFamilies                []string       `json:"tool_families"`
	DelegationKinds             []string       `json:"delegation_kinds"`
	CanMessage                  []string       `json:"can_message"`
	SpecialistSummaryVisibility string         `json:"specialist_summary_visibility"`
	SoulExtra                   map[string]any `json:"soul_extra"`
	IsFront                     bool           `json:"is_front"`
}

type teamProfileRequest struct {
	ProfileID string `json:"profile_id"`
}

type teamCloneRequest struct {
	SourceProfileID string `json:"source_profile_id"`
	ProfileID       string `json:"profile_id"`
}

type teamImportRequest struct {
	YAML string `json:"yaml"`
}

type teamSaveRequest struct {
	Team teamConfigInput `json:"team"`
}

type teamConfigInput struct {
	Name         string            `json:"name"`
	FrontAgentID string            `json:"front_agent_id"`
	Members      []teamMemberInput `json:"members"`
}

type teamMemberInput struct {
	ID                          string         `json:"id"`
	Role                        string         `json:"role"`
	SoulFile                    string         `json:"soul_file"`
	BaseProfile                 string         `json:"base_profile"`
	ToolFamilies                []string       `json:"tool_families"`
	DelegationKinds             []string       `json:"delegation_kinds"`
	CanMessage                  []string       `json:"can_message"`
	SpecialistSummaryVisibility string         `json:"specialist_summary_visibility"`
	SoulExtra                   map[string]any `json:"soul_extra"`
}

func (s *Server) handleTeamAPI(w http.ResponseWriter, r *http.Request) {
	state, err := s.loadTeamPageState(r.Context())
	if err != nil {
		http.Error(w, "failed to load team surface", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, buildTeamAPIResponse(state, ""))
}

func (s *Server) handleTeamSelectAPI(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	var req teamProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	profileID := strings.TrimSpace(req.ProfileID)
	if profileID == "" {
		http.Error(w, "profile_id is required", http.StatusBadRequest)
		return
	}
	if err := s.rt.SelectTeamProfile(r.Context(), profileID); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": err.Error()})
		return
	}
	state, err := s.loadTeamPageState(r.Context())
	if err != nil {
		http.Error(w, "failed to reload team surface", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, buildTeamAPIResponse(state, fmt.Sprintf("Active profile switched to %s.", profileID)))
}

func (s *Server) handleTeamCreateAPI(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	var req teamProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	profileID := strings.TrimSpace(req.ProfileID)
	if profileID == "" {
		http.Error(w, "profile_id is required", http.StatusBadRequest)
		return
	}
	if err := s.rt.CreateTeamProfile(r.Context(), profileID); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": err.Error()})
		return
	}
	if err := s.rt.SelectTeamProfile(r.Context(), profileID); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": err.Error()})
		return
	}
	state, err := s.loadTeamPageState(r.Context())
	if err != nil {
		http.Error(w, "failed to reload team surface", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, buildTeamAPIResponse(state, fmt.Sprintf("Profile %s created and selected.", profileID)))
}

func (s *Server) handleTeamCloneAPI(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	var req teamCloneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	sourceProfileID := strings.TrimSpace(req.SourceProfileID)
	profileID := strings.TrimSpace(req.ProfileID)
	if sourceProfileID == "" || profileID == "" {
		http.Error(w, "source_profile_id and profile_id are required", http.StatusBadRequest)
		return
	}
	if err := s.rt.CloneTeamProfile(r.Context(), sourceProfileID, profileID); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": err.Error()})
		return
	}
	if err := s.rt.SelectTeamProfile(r.Context(), profileID); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": err.Error()})
		return
	}
	state, err := s.loadTeamPageState(r.Context())
	if err != nil {
		http.Error(w, "failed to reload team surface", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, buildTeamAPIResponse(state, fmt.Sprintf("Profile %s cloned from %s.", profileID, sourceProfileID)))
}

func (s *Server) handleTeamDeleteAPI(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	var req teamProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	profileID := strings.TrimSpace(req.ProfileID)
	if profileID == "" {
		http.Error(w, "profile_id is required", http.StatusBadRequest)
		return
	}
	if err := s.rt.DeleteTeamProfile(r.Context(), profileID); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": err.Error()})
		return
	}
	state, err := s.loadTeamPageState(r.Context())
	if err != nil {
		http.Error(w, "failed to reload team surface", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, buildTeamAPIResponse(state, fmt.Sprintf("Profile %s deleted.", profileID)))
}

func (s *Server) handleTeamSaveAPI(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	var req teamSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	cfg := teamConfigFromAPIInput(req.Team)
	if err := s.rt.UpdateTeam(r.Context(), cfg); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": err.Error()})
		return
	}
	state, err := s.loadTeamPageState(r.Context())
	if err != nil {
		http.Error(w, "failed to reload team surface", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, buildTeamAPIResponse(state, "Team saved."))
}

func (s *Server) handleTeamImportAPI(w http.ResponseWriter, r *http.Request) {
	var req teamImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	cfg, err := teams.LoadEditableYAML([]byte(req.YAML))
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": err.Error()})
		return
	}
	state, err := s.loadTeamPageState(r.Context())
	if err != nil {
		http.Error(w, "failed to load team surface", http.StatusInternalServerError)
		return
	}
	state.Config = cfg
	writeJSON(w, http.StatusOK, buildTeamAPIResponse(state, "Imported file loaded. Save Team to apply the change."))
}

func buildTeamAPIResponse(state teamPageState, notice string) teamAPIResponse {
	activeProfileID := state.ActiveProfile
	if activeProfileID == "" {
		activeProfileID = teams.DefaultProfileName
	}
	resp := teamAPIResponse{
		Notice: notice,
		ActiveProfile: teamProfileResponse{
			ID:       activeProfileID,
			Label:    activeProfileID,
			Active:   true,
			SavePath: state.ProfileSavePath,
		},
		Profiles: make([]teamProfileResponse, 0, len(state.Profiles)),
		Team: teamConfigResponse{
			Name:         state.Config.Name,
			FrontAgentID: state.Config.FrontAgent,
			MemberCount:  len(state.Config.Agents),
			Members:      make([]teamMemberResponse, 0, len(state.Config.Agents)),
		},
	}

	seenProfiles := map[string]bool{
		activeProfileID: true,
	}
	resp.Profiles = append(resp.Profiles, teamProfileResponse{
		ID:     activeProfileID,
		Label:  activeProfileID,
		Active: true,
	})
	for _, profile := range state.Profiles {
		if seenProfiles[profile.Name] {
			continue
		}
		seenProfiles[profile.Name] = true
		resp.Profiles = append(resp.Profiles, teamProfileResponse{
			ID:     profile.Name,
			Label:  profile.Name,
			Active: profile.Name == activeProfileID,
		})
	}

	for _, agent := range state.Config.Agents {
		resp.Team.Members = append(resp.Team.Members, teamMemberResponse{
			ID:                          agent.ID,
			Role:                        agent.Role,
			SoulFile:                    agent.SoulFile,
			BaseProfile:                 string(agent.BaseProfile),
			ToolFamilies:                toolFamilyStrings(agent.ToolFamilies),
			DelegationKinds:             delegationKindStrings(agent.DelegationKinds),
			CanMessage:                  append([]string(nil), agent.CanMessage...),
			SpecialistSummaryVisibility: string(agent.SpecialistSummaryVisibility),
			SoulExtra:                   cloneSoulExtra(agent.Soul.Extra),
			IsFront:                     agent.ID == state.Config.FrontAgent,
		})
	}

	return resp
}

func teamConfigFromAPIInput(input teamConfigInput) teams.Config {
	cfg := teams.Config{
		Name:       strings.TrimSpace(input.Name),
		FrontAgent: strings.TrimSpace(input.FrontAgentID),
		Agents:     make([]teams.AgentConfig, 0, len(input.Members)),
	}
	for _, member := range input.Members {
		memberID := strings.TrimSpace(member.ID)
		soulFile := strings.TrimSpace(member.SoulFile)
		if soulFile == "" {
			soulFile = teams.SuggestedSoulFile(memberID)
		}
		role := strings.TrimSpace(member.Role)
		cfg.Agents = append(cfg.Agents, teams.AgentConfig{
			ID:                          memberID,
			SoulFile:                    soulFile,
			Role:                        role,
			BaseProfile:                 model.BaseProfile(strings.TrimSpace(member.BaseProfile)),
			ToolFamilies:                normalizeToolFamilies(member.ToolFamilies),
			DelegationKinds:             normalizeDelegationKinds(member.DelegationKinds),
			CanMessage:                  normalizeAgentLinks(member.CanMessage),
			SpecialistSummaryVisibility: model.SpecialistSummaryVisibility(strings.TrimSpace(member.SpecialistSummaryVisibility)),
			Soul: teams.SoulSpec{
				Role:  role,
				Extra: cloneSoulExtra(member.SoulExtra),
			},
		})
	}
	return cfg
}

func toolFamilyStrings(values []model.ToolFamily) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, string(value))
	}
	return items
}

func delegationKindStrings(values []model.DelegationKind) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, string(value))
	}
	return items
}

func cloneSoulExtra(extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(extra))
	for key, value := range extra {
		cloned[key] = value
	}
	return cloned
}
