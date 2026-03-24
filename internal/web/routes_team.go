package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

// ── Soul types ───────────────────────────────────────────────────────────────

// soulSpec mirrors the typed fields of a soul YAML file. No raw_prompt field exists.
type soulSpec struct {
	Role               string   `yaml:"role"`
	Tone               string   `yaml:"tone"`
	Posture            string   `yaml:"posture"`
	CollaborationStyle string   `yaml:"collaboration_style"`
	EscalationRules    []string `yaml:"escalation_rules"`
	DecisionBoundaries []string `yaml:"decision_boundaries"`
	ToolPosture        string   `yaml:"tool_posture"`
	Prohibitions       []string `yaml:"prohibitions"`
	Notes              string   `yaml:"notes"`
}

// ── Page data ────────────────────────────────────────────────────────────────

type teamsListPageData struct {
	Teams []teamSummary
}

type teamSummary struct {
	ID         string
	Name       string
	AgentCount int
}

type soulEditorPageData struct {
	TeamID  string
	AgentID string
	Soul    soulSpec
	Error   string
	Field   string // the field that failed validation
}

type composerPageData struct {
	TeamID string
	Agents []composerAgent
	Edges  []runtime.HandoffEdge
	Error  string
}

type composerAgent struct {
	ID           string
	SoulFile     string
	Capabilities []string
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (s *Server) handleTeamsList(w http.ResponseWriter, r *http.Request) {
	if len(s.teamDirs) == 0 {
		http.Error(w, "team directory not configured", http.StatusServiceUnavailable)
		return
	}
	teams := make([]teamSummary, 0, len(s.teamDirs))
	for id, dir := range s.teamDirs {
		spec, err := s.cachedTeamSpec(dir)
		if err != nil {
			http.Error(w, fmt.Sprintf("load team %s: %v", id, err), http.StatusInternalServerError)
			return
		}
		teams = append(teams, teamSummary{
			ID:         id,
			Name:       spec.Name,
			AgentCount: len(spec.Agents),
		})
	}
	s.renderTemplate(w, "Teams", "teams_list_body", teamsListPageData{Teams: teams})
}

func (s *Server) handleSoulEditor(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	agentID := r.PathValue("agent")

	soul, soulPath, err := s.loadSoul(teamID, agentID)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, fmt.Sprintf("load soul: %v", err), http.StatusInternalServerError)
		return
	}
	_ = soulPath

	data := soulEditorPageData{
		TeamID:  teamID,
		AgentID: agentID,
		Soul:    *soul,
	}
	s.renderTemplate(w, "Soul Editor", "soul_editor_body", data)
}

func (s *Server) handleSoulUpdate(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	agentID := r.PathValue("agent")

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	field := strings.TrimSpace(r.FormValue("field"))
	value := strings.TrimSpace(r.FormValue("value"))

	// Validate: required scalar fields must not be blank.
	requiredScalars := map[string]bool{
		"role": true, "tone": true, "posture": true,
		"collaboration_style": true, "tool_posture": true,
	}
	if requiredScalars[field] && value == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		data := soulEditorPageData{
			TeamID:  teamID,
			AgentID: agentID,
			Error:   fmt.Sprintf("field %q is required and must not be empty", field),
			Field:   field,
		}
		s.renderTemplate(w, "Soul Editor", "soul_editor_body", data)
		return
	}

	// Load soul outside the lock: cachedTeamSpec acquires teamMu internally.
	soul, soulPath, err := s.loadSoul(teamID, agentID)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, fmt.Sprintf("load soul: %v", err), http.StatusInternalServerError)
		return
	}

	// Mutate only the submitted field.
	switch field {
	case "role":
		soul.Role = value
	case "tone":
		soul.Tone = value
	case "posture":
		soul.Posture = value
	case "collaboration_style":
		soul.CollaborationStyle = value
	case "tool_posture":
		soul.ToolPosture = value
	case "notes":
		soul.Notes = value
	case "escalation_rules":
		soul.EscalationRules = splitLines(value)
	case "decision_boundaries":
		soul.DecisionBoundaries = splitLines(value)
	case "prohibitions":
		soul.Prohibitions = splitLines(value)
	default:
		http.Error(w, fmt.Sprintf("unknown field %q", field), http.StatusBadRequest)
		return
	}

	out, err := yaml.Marshal(soul)
	if err != nil {
		http.Error(w, fmt.Sprintf("marshal soul: %v", err), http.StatusInternalServerError)
		return
	}

	// Lock only for the file write to serialise concurrent soul updates.
	s.teamMu.Lock()
	writeErr := os.WriteFile(soulPath, out, 0644)
	s.teamMu.Unlock()

	if writeErr != nil {
		http.Error(w, fmt.Sprintf("write soul: %v", writeErr), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/teams/%s/soul/%s", teamID, agentID), http.StatusSeeOther)
}

func (s *Server) handleComposer(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	spec, err := s.loadTeamSpecForID(teamID)
	if err != nil {
		http.Error(w, fmt.Sprintf("load team: %v", err), http.StatusInternalServerError)
		return
	}

	agents := make([]composerAgent, 0, len(spec.Agents))
	for _, a := range spec.Agents {
		caps := spec.CapabilityFlags[a.ID]
		if caps == nil {
			caps = []string{}
		}
		agents = append(agents, composerAgent{
			ID:           a.ID,
			SoulFile:     a.SoulFile,
			Capabilities: caps,
		})
	}

	data := composerPageData{
		TeamID: teamID,
		Agents: agents,
		Edges:  spec.HandoffEdges,
	}
	s.renderTemplate(w, "Team Composer", "team_composer_body", data)
}

func (s *Server) handleComposerMutate(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	action := r.FormValue("action")

	// Load team spec outside the lock: cachedTeamSpec acquires teamMu internally.
	spec, err := s.loadTeamSpecForID(teamID)
	if err != nil {
		http.Error(w, fmt.Sprintf("load team: %v", err), http.StatusInternalServerError)
		return
	}

	switch action {
	case "add_agent":
		agentID := strings.TrimSpace(r.FormValue("agent_id"))
		soulFile := strings.TrimSpace(r.FormValue("soul_file"))
		capability := strings.TrimSpace(r.FormValue("capability"))

		if !isAllowedCapabilityFlag(capability) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprintf(w, "unknown capability flag %q", capability)
			return
		}

		// Check not already declared.
		for _, a := range spec.Agents {
			if a.ID == agentID {
				http.Error(w, fmt.Sprintf("agent %q already declared", agentID), http.StatusConflict)
				return
			}
		}

		spec.Agents = append(spec.Agents, runtime.AgentSpec{ID: agentID, SoulFile: soulFile})
		if spec.CapabilityFlags == nil {
			spec.CapabilityFlags = make(map[string][]string)
		}
		spec.CapabilityFlags[agentID] = []string{capability}

	case "wire_edge":
		from := strings.TrimSpace(r.FormValue("from"))
		to := strings.TrimSpace(r.FormValue("to"))

		// Self-referential edges are circular.
		if from == to {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprintf(w, "circular handoff: from and to must be different agents")
			return
		}

		// Check for circular chains: if adding from->to would create a cycle,
		// detect it by seeing if 'from' is already reachable from 'to'.
		if reachable(spec.HandoffEdges, to, from) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprintf(w, "circular handoff chain: adding %s->%s would create a cycle", from, to)
			return
		}

		spec.HandoffEdges = append(spec.HandoffEdges, runtime.HandoffEdge{From: from, To: to})

	default:
		http.Error(w, fmt.Sprintf("unknown action %q", action), http.StatusBadRequest)
		return
	}

	// Validate the proposed spec before writing.
	raw, err := yaml.Marshal(spec)
	if err != nil {
		http.Error(w, fmt.Sprintf("marshal team: %v", err), http.StatusInternalServerError)
		return
	}
	if _, err := runtime.LoadTeamSpec(raw); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprintf(w, "team validation failed: %v", err)
		return
	}

	teamDir := s.teamDirs[teamID]
	teamPath := filepath.Join(teamDir, "team.yaml")

	// Lock only for the file write to serialise concurrent team mutations.
	s.teamMu.Lock()
	writeErr := os.WriteFile(teamPath, raw, 0644)
	s.teamMu.Unlock()

	if writeErr != nil {
		http.Error(w, fmt.Sprintf("write team: %v", writeErr), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/teams/%s/composer", teamID), http.StatusSeeOther)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func isAllowedCapabilityFlag(flag string) bool {
	return model.IsValidCapability(flag)
}

// reachable returns true if target is reachable from start via the given edges.
func reachable(edges []runtime.HandoffEdge, start, target string) bool {
	visited := map[string]bool{}
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == target {
			return true
		}
		if visited[cur] {
			continue
		}
		visited[cur] = true
		for _, e := range edges {
			if e.From == cur {
				queue = append(queue, e.To)
			}
		}
	}
	return false
}

// loadSoul finds the soul file for agentID in teamID and parses it.
// Returns the soul spec, the absolute path to the file, and any error.
func (s *Server) loadSoul(teamID, agentID string) (*soulSpec, string, error) {
	spec, err := s.loadTeamSpecForID(teamID)
	if err != nil {
		return nil, "", err
	}

	var soulFile string
	for _, a := range spec.Agents {
		if a.ID == agentID {
			soulFile = a.SoulFile
			break
		}
	}
	if soulFile == "" {
		return nil, "", os.ErrNotExist
	}

	soulPath := filepath.Join(s.teamDirs[teamID], soulFile)
	data, err := os.ReadFile(soulPath)
	if err != nil {
		return nil, "", err
	}

	var soul soulSpec
	if err := yaml.Unmarshal(data, &soul); err != nil {
		return nil, "", fmt.Errorf("parse soul: %w", err)
	}
	return &soul, soulPath, nil
}

// loadTeamSpecForID loads and validates the team.yaml for the given team ID.
func (s *Server) loadTeamSpecForID(teamID string) (*runtime.TeamSpec, error) {
	dir, ok := s.teamDirs[teamID]
	if !ok {
		return nil, fmt.Errorf("team %q not found", teamID)
	}
	return s.cachedTeamSpec(dir)
}

// cachedTeamSpec returns the parsed TeamSpec for teamDir, re-reading only if
// team.yaml has been modified since the last parse.
// Caller must NOT hold teamMu — this method acquires it internally.
func (s *Server) cachedTeamSpec(teamDir string) (*runtime.TeamSpec, error) {
	path := filepath.Join(teamDir, "team.yaml")

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat team.yaml: %w", err)
	}
	mtime := info.ModTime()

	s.teamMu.Lock()
	entry, hit := s.teamCache[teamDir]
	s.teamMu.Unlock()

	if hit && !entry.mtime.Before(mtime) {
		return entry.spec, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read team.yaml: %w", err)
	}
	spec, err := runtime.LoadTeamSpec(data)
	if err != nil {
		return nil, err
	}

	s.teamMu.Lock()
	s.teamCache[teamDir] = teamSpecCacheEntry{spec: spec, mtime: mtime}
	s.teamMu.Unlock()

	return spec, nil
}

// splitLines splits a multi-line string into trimmed non-empty lines.
func splitLines(s string) []string {
	var result []string
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			result = append(result, t)
		}
	}
	return result
}
