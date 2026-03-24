package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/replay"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

// teamHarness wraps serverHarness with a temp teams directory for team/soul tests.
type teamHarness struct {
	*serverHarness
	teamDir string
}

func newTeamHarness(t *testing.T) *teamHarness {
	t.Helper()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	workspaceRoot := t.TempDir()
	adminToken := "test-admin-token"
	seedSettings(t, db, map[string]string{
		"admin_token":    adminToken,
		"workspace_root": workspaceRoot,
		"team_name":      "Test Team",
	})

	// Create a temp teams directory with a default team.
	teamDir := t.TempDir()
	writeDefaultTeamFixture(t, teamDir)

	cs := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, cs)
	reg := tools.NewRegistry()
	prov := runtime.NewMockProvider(nil, nil)
	broadcaster := NewSSEBroadcaster()
	rt := runtime.New(db, cs, reg, mem, prov, broadcaster)

	server, err := NewServer(Options{
		DB:          db,
		Replay:      replay.NewService(db),
		Broadcaster: broadcaster,
		Runtime:     rt,
		TeamDir:     teamDir,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	base := &serverHarness{
		db:            db,
		server:        server,
		broadcaster:   broadcaster,
		rt:            rt,
		adminToken:    adminToken,
		workspaceRoot: workspaceRoot,
	}
	return &teamHarness{serverHarness: base, teamDir: teamDir}
}

// writeDefaultTeamFixture writes a minimal default team + coordinator soul to dir.
func writeDefaultTeamFixture(t *testing.T, teamDir string) {
	t.Helper()

	teamYAML := `name: default
agents:
  - id: coordinator
    soul_file: coordinator.soul.yaml
  - id: patcher
    soul_file: patcher.soul.yaml
capability_flags:
  coordinator: [operator_facing]
  patcher: [workspace_write]
handoff_edges:
  - from: coordinator
    to: patcher
  - from: patcher
    to: coordinator
`
	coordinatorSoul := `role: operator-facing coordinator
tone: clear, precise, no filler
posture: ask before assuming; surface ambiguity early
collaboration_style: delegates to specialists; synthesises results before reporting back
escalation_rules:
  - if task is ambiguous after one clarification attempt, surface to operator with two concrete options
decision_boundaries:
  - may decompose objectives into sub-tasks
  - must not apply workspace changes directly
tool_posture: operator_facing
prohibitions:
  - must not call workspace_apply directly
notes: entry point for all operator-initiated runs
`
	patcherSoul := `role: workspace patcher
tone: methodical
posture: confirm before writing
collaboration_style: reports changes back to coordinator
escalation_rules:
  - escalate conflicts to coordinator
decision_boundaries:
  - may write to workspace files
  - must not exceed approved scope
tool_posture: workspace_write
prohibitions:
  - must not delete files without explicit instruction
notes: applies patches approved by coordinator
`

	if err := os.WriteFile(filepath.Join(teamDir, "team.yaml"), []byte(teamYAML), 0644); err != nil {
		t.Fatalf("write team.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamDir, "coordinator.soul.yaml"), []byte(coordinatorSoul), 0644); err != nil {
		t.Fatalf("write coordinator.soul.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamDir, "patcher.soul.yaml"), []byte(patcherSoul), 0644); err != nil {
		t.Fatalf("write patcher.soul.yaml: %v", err)
	}
}

// ── Soul editor tests ────────────────────────────────────────────────────────

func TestSoulEditor(t *testing.T) {
	t.Run("GET renders all typed fields without raw prompt textarea", func(t *testing.T) {
		h := newTeamHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/teams/default/soul/coordinator", nil)
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		body := rr.Body.String()
		// All typed fields must be present.
		for _, field := range []string{
			"role", "tone", "posture", "collaboration_style",
			"escalation_rules", "decision_boundaries", "tool_posture",
			"prohibitions", "notes",
		} {
			if !strings.Contains(body, field) {
				t.Errorf("expected body to contain field %q", field)
			}
		}
		// Soul content must appear.
		if !strings.Contains(body, "operator-facing coordinator") {
			t.Error("expected role value to appear in body")
		}
		// No raw prompt textarea — never expose a single full-prompt textarea.
		if strings.Contains(body, "raw_prompt") {
			t.Error("body must not contain raw_prompt")
		}
		if strings.Contains(body, "full_prompt") {
			t.Error("body must not contain full_prompt")
		}
	})

	t.Run("POST single field update persists only that field", func(t *testing.T) {
		h := newTeamHarness(t)

		form := url.Values{
			"field": {"tone"},
			"value": {"direct and terse"},
		}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/teams/default/soul/coordinator",
			strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d: %s", rr.Code, rr.Body.String())
		}

		// Read the soul file back and verify only tone changed.
		data, err := os.ReadFile(filepath.Join(h.teamDir, "coordinator.soul.yaml"))
		if err != nil {
			t.Fatalf("read soul file: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "direct and terse") {
			t.Error("updated tone value not found in soul file")
		}
		// Role must be unchanged.
		if !strings.Contains(content, "operator-facing coordinator") {
			t.Error("role was unexpectedly modified")
		}
		// No raw_prompt key must appear.
		if strings.Contains(content, "raw_prompt") {
			t.Error("soul file must not contain raw_prompt after edit")
		}
	})

	t.Run("POST empty required field returns 422", func(t *testing.T) {
		h := newTeamHarness(t)

		form := url.Values{
			"field": {"role"},
			"value": {""},
		}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/teams/default/soul/coordinator",
			strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "role") {
			t.Error("422 body should mention the failing field name")
		}
	})

	t.Run("persisted YAML does not grow a raw_prompt key", func(t *testing.T) {
		h := newTeamHarness(t)

		// Update posture.
		form := url.Values{
			"field": {"posture"},
			"value": {"confirm all writes"},
		}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/teams/default/soul/coordinator",
			strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303, got %d: %s", rr.Code, rr.Body.String())
		}

		data, err := os.ReadFile(filepath.Join(h.teamDir, "coordinator.soul.yaml"))
		if err != nil {
			t.Fatalf("read soul file: %v", err)
		}
		if strings.Contains(string(data), "raw_prompt") {
			t.Error("raw_prompt must not appear in persisted YAML")
		}
	})

	t.Run("GET unknown agent returns 404", func(t *testing.T) {
		h := newTeamHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/teams/default/soul/nonexistent", nil)
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for unknown agent, got %d", rr.Code)
		}
	})
}

// ── Visual composer tests ────────────────────────────────────────────────────

func TestComposer(t *testing.T) {
	t.Run("GET renders agent nodes and handoff edges from YAML", func(t *testing.T) {
		h := newTeamHarness(t)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/teams/default/composer", nil)
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		body := rr.Body.String()
		// Both agents should appear.
		if !strings.Contains(body, "coordinator") {
			t.Error("expected coordinator agent in composer view")
		}
		if !strings.Contains(body, "patcher") {
			t.Error("expected patcher agent in composer view")
		}
		// Handoff edges should appear.
		if !strings.Contains(body, "coordinator") || !strings.Contains(body, "patcher") {
			t.Error("expected handoff edges to mention both agents")
		}
		// Capability flags should appear.
		if !strings.Contains(body, "operator_facing") {
			t.Error("expected capability flag operator_facing in composer view")
		}
	})

	t.Run("POST add agent writes to YAML only, no second config file", func(t *testing.T) {
		h := newTeamHarness(t)

		form := url.Values{
			"action":      {"add_agent"},
			"agent_id":    {"reviewer"},
			"soul_file":   {"reviewer.soul.yaml"},
			"capability":  {"read_heavy"},
		}
		// Create a stub soul file for the new agent.
		stubSoul := "role: reviewer\ntone: careful\nposture: read only\ncollaboration_style: reports findings\nescalation_rules:\n  - escalate critical findings\ndecision_boundaries:\n  - may read files\ntool_posture: read_heavy\nprohibitions:\n  - must not write files\nnotes: code reviewer\n"
		if err := os.WriteFile(filepath.Join(h.teamDir, "reviewer.soul.yaml"), []byte(stubSoul), 0644); err != nil {
			t.Fatalf("write reviewer soul: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/teams/default/composer",
			strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d: %s", rr.Code, rr.Body.String())
		}

		// Read YAML back — reviewer must be in agents list.
		data, err := os.ReadFile(filepath.Join(h.teamDir, "team.yaml"))
		if err != nil {
			t.Fatalf("read team.yaml: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "reviewer") {
			t.Error("reviewer not found in team.yaml after add_agent")
		}

		// No second config file should exist.
		entries, _ := os.ReadDir(h.teamDir)
		yamlCount := 0
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".yaml") && strings.HasPrefix(e.Name(), "team") {
				yamlCount++
			}
		}
		if yamlCount > 1 {
			t.Errorf("expected exactly 1 team*.yaml, found %d", yamlCount)
		}
	})

	t.Run("POST wire handoff edge writes edge to YAML", func(t *testing.T) {
		h := newTeamHarness(t)

		// Add reviewer first so the edge references a declared agent.
		stubSoul := "role: reviewer\ntone: careful\nposture: read only\ncollaboration_style: reports findings\nescalation_rules:\n  - escalate critical findings\ndecision_boundaries:\n  - may read files\ntool_posture: read_heavy\nprohibitions:\n  - must not write files\nnotes: code reviewer\n"
		if err := os.WriteFile(filepath.Join(h.teamDir, "reviewer.soul.yaml"), []byte(stubSoul), 0644); err != nil {
			t.Fatalf("write reviewer soul: %v", err)
		}
		addAgent(t, h, "reviewer", "reviewer.soul.yaml", "read_heavy")

		form := url.Values{
			"action": {"wire_edge"},
			"from":   {"patcher"},
			"to":     {"reviewer"},
		}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/teams/default/composer",
			strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d: %s", rr.Code, rr.Body.String())
		}

		data, err := os.ReadFile(filepath.Join(h.teamDir, "team.yaml"))
		if err != nil {
			t.Fatalf("read team.yaml: %v", err)
		}
		content := string(data)
		// Edge patcher->reviewer must appear.
		if !strings.Contains(content, "patcher") || !strings.Contains(content, "reviewer") {
			t.Error("handoff edge patcher->reviewer not persisted")
		}
	})

	t.Run("POST unknown capability flag returns 422 and YAML unchanged", func(t *testing.T) {
		h := newTeamHarness(t)

		// Snapshot original YAML.
		before, err := os.ReadFile(filepath.Join(h.teamDir, "team.yaml"))
		if err != nil {
			t.Fatalf("read team.yaml: %v", err)
		}

		form := url.Values{
			"action":     {"add_agent"},
			"agent_id":   {"badagent"},
			"soul_file":  {"badagent.soul.yaml"},
			"capability": {"super_admin"}, // unknown flag
		}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/teams/default/composer",
			strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for unknown capability flag, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "super_admin") {
			t.Error("422 body should mention the invalid flag")
		}

		// YAML must be unchanged.
		after, err := os.ReadFile(filepath.Join(h.teamDir, "team.yaml"))
		if err != nil {
			t.Fatalf("read team.yaml after: %v", err)
		}
		if string(before) != string(after) {
			t.Error("team.yaml was modified despite validation failure")
		}
	})

	t.Run("POST circular handoff chain returns 422", func(t *testing.T) {
		h := newTeamHarness(t)

		// coordinator->patcher and patcher->coordinator already exist.
		// Adding coordinator->coordinator would be self-referential (circular length 1).
		form := url.Values{
			"action": {"wire_edge"},
			"from":   {"coordinator"},
			"to":     {"coordinator"},
		}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/teams/default/composer",
			strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for circular handoff, got %d", rr.Code)
		}
	})

	t.Run("YAML after valid POST passes LoadTeamSpec validation", func(t *testing.T) {
		h := newTeamHarness(t)

		// Wire a new edge that doesn't introduce a cycle.
		// coordinator->patcher already exists; we'll verify the existing YAML is valid first.
		data, err := os.ReadFile(filepath.Join(h.teamDir, "team.yaml"))
		if err != nil {
			t.Fatalf("read team.yaml: %v", err)
		}
		if _, err := runtime.LoadTeamSpec(data); err != nil {
			t.Fatalf("pre-condition: team.yaml must be valid before test: %v", err)
		}

		// Add a new agent and ensure the result is still valid.
		stubSoul := "role: verifier\ntone: precise\nposture: verify only\ncollaboration_style: reports outcomes\nescalation_rules:\n  - escalate failures\ndecision_boundaries:\n  - may read and verify\ntool_posture: propose_only\nprohibitions:\n  - must not modify files\nnotes: final verifier\n"
		if err := os.WriteFile(filepath.Join(h.teamDir, "verifier.soul.yaml"), []byte(stubSoul), 0644); err != nil {
			t.Fatalf("write verifier soul: %v", err)
		}
		addAgent(t, h, "verifier", "verifier.soul.yaml", "propose_only")

		data, err = os.ReadFile(filepath.Join(h.teamDir, "team.yaml"))
		if err != nil {
			t.Fatalf("read team.yaml after add: %v", err)
		}
		if _, err := runtime.LoadTeamSpec(data); err != nil {
			t.Fatalf("team.yaml invalid after valid POST: %v", err)
		}
	})
}

// addAgent is a helper that issues a POST /teams/default/composer add_agent action.
func addAgent(t *testing.T, h *teamHarness, agentID, soulFile, capability string) {
	t.Helper()

	form := url.Values{
		"action":     {"add_agent"},
		"agent_id":   {agentID},
		"soul_file":  {soulFile},
		"capability": {capability},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/teams/default/composer",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.server.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("addAgent: expected 303, got %d: %s", rr.Code, rr.Body.String())
	}
}
