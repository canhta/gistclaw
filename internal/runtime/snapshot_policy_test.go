package runtime

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/memory"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
	"github.com/canhta/gistclaw/internal/tools"
)

func TestRunEngine_UsesExecutionSnapshotToolProfileForPolicy(t *testing.T) {
	db, cs, mem, _ := setupRunTestDeps(t)
	registry, closer, err := buildRuntimeRepoRegistry()
	if err != nil {
		t.Fatalf("buildRuntimeRepoRegistry: %v", err)
	}
	if closer != nil {
		defer closer.Close()
	}
	prov := NewMockProvider(
		[]GenerateResult{
			{
				ToolCalls: []model.ToolCallRequest{
					{ID: "call-write", ToolName: "write_new_file", InputJSON: []byte(`{"path":"new.txt","content":"hello"}`)},
				},
				StopReason: "tool_calls",
			},
			{Content: "done", StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, registry, mem, prov, &model.NoopEventSink{})

	run, err := rt.Start(context.Background(), StartRun{
		ConversationID:        "conv-snapshot-policy",
		AgentID:               "reviewer",
		Objective:             "attempt write",
		CWD:         t.TempDir(),
		ExecutionSnapshotJSON: mustSnapshotJSON(t, model.ExecutionSnapshot{TeamID: "default", Agents: map[string]model.AgentProfile{"reviewer": {AgentID: "reviewer", ToolProfile: "read_heavy", Capabilities: []model.AgentCapability{model.CapReadHeavy}}}}),
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var decision string
	var outputJSON []byte
	err = db.RawDB().QueryRowContext(context.Background(),
		"SELECT decision, output_json FROM tool_calls WHERE run_id = ? AND tool_name = 'write_new_file' LIMIT 1",
		run.ID,
	).Scan(&decision, &outputJSON)
	if err != nil {
		t.Fatalf("query tool call: %v", err)
	}
	if decision != string(model.DecisionDeny) {
		t.Fatalf("expected deny decision, got %q", decision)
	}
	var result model.ToolResult
	if err := json.Unmarshal(outputJSON, &result); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}
	if result.Error == "" {
		t.Fatalf("expected denial error, got %+v", result)
	}
}

func TestRuntime_DefaultExecutionSnapshotIsAppliedToChildSpawns(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cs := conversations.NewConversationStore(db)
	mem := memory.NewStore(db, cs)
	reg, closer, err := buildRuntimeRepoRegistry()
	if err != nil {
		t.Fatalf("buildRuntimeRepoRegistry: %v", err)
	}
	if closer != nil {
		defer closer.Close()
	}
	prov := NewMockProvider(
		[]GenerateResult{
			{Content: "assistant done", StopReason: "end_turn"},
			{
				ToolCalls: []model.ToolCallRequest{
					{ID: "call-child-write", ToolName: "write_new_file", InputJSON: []byte(`{"path":"child.txt","content":"nope"}`)},
				},
				StopReason: "tool_calls",
			},
			{Content: "child done", StopReason: "end_turn"},
		},
		nil,
	)
	rt := New(db, cs, reg, mem, prov, &model.NoopEventSink{})
	rt.SetDefaultExecutionSnapshot(model.ExecutionSnapshot{
		TeamID: "default",
		Agents: map[string]model.AgentProfile{
			"assistant": {AgentID: "assistant", ToolProfile: "operator_facing", Capabilities: []model.AgentCapability{model.CapOperatorFacing, model.CapSpawn}},
			"reviewer":  {AgentID: "reviewer", ToolProfile: "read_heavy", Capabilities: []model.AgentCapability{model.CapReadHeavy}},
		},
	})

	parent, err := rt.StartFrontSession(context.Background(), StartFrontSession{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: "web",
			AccountID:   "local",
			ExternalID:  "assistant",
			ThreadID:    "main",
		},
		FrontAgentID:  "assistant",
		InitialPrompt: "start",
		CWD: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartFrontSession failed: %v", err)
	}
	child, err := rt.Spawn(context.Background(), SpawnCommand{
		ControllerSessionID: parent.SessionID,
		AgentID:             "reviewer",
		Prompt:              "attempt write",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	var teamID string
	var snapshotJSON []byte
	if err := db.RawDB().QueryRow(
		"SELECT COALESCE(team_id, ''), execution_snapshot_json FROM runs WHERE id = ?",
		child.ID,
	).Scan(&teamID, &snapshotJSON); err != nil {
		t.Fatalf("query child run: %v", err)
	}
	if teamID != "default" {
		t.Fatalf("expected child team_id default, got %q", teamID)
	}
	if len(snapshotJSON) == 0 {
		t.Fatal("expected child execution snapshot to be persisted")
	}

	var decision string
	if err := db.RawDB().QueryRow(
		"SELECT decision FROM tool_calls WHERE run_id = ? AND tool_name = 'write_new_file' LIMIT 1",
		child.ID,
	).Scan(&decision); err != nil {
		t.Fatalf("query child tool call: %v", err)
	}
	if decision != string(model.DecisionDeny) {
		t.Fatalf("expected child reviewer write to be denied, got %q", decision)
	}
}

func buildRuntimeRepoRegistry() (*tools.Registry, io.Closer, error) {
	return tools.BuildRegistry(context.Background(), tools.BuildOptions{})
}

func mustSnapshotJSON(t *testing.T, snapshot model.ExecutionSnapshot) []byte {
	t.Helper()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return raw
}
