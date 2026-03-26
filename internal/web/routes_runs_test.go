package web

import (
	"strings"
	"testing"
	"time"
)

func TestHumanizeRunRelativeTime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 26, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		when time.Time
		want string
	}{
		{name: "just now", when: now.Add(-20 * time.Second), want: "just now"},
		{name: "minutes", when: now.Add(-5 * time.Minute), want: "5m ago"},
		{name: "hours", when: now.Add(-3 * time.Hour), want: "3h ago"},
		{name: "days", when: now.Add(-48 * time.Hour), want: "2d ago"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := humanizeRunRelativeTime(now, tc.when); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSummarizeRunBlocker(t *testing.T) {
	t.Parallel()

	rows := []runChildRow{
		{AgentID: "patcher", Status: "needs_approval"},
		{AgentID: "reviewer", Status: "active"},
	}

	cases := []struct {
		name        string
		queueStatus string
		rows        []runChildRow
		rootStatus  string
		childCount  int
		want        string
	}{
		{name: "approval", queueStatus: "needs_approval", rows: rows, rootStatus: "active", childCount: 2, want: "patcher waiting on approval"},
		{name: "failure without agent", queueStatus: "failed", rows: []runChildRow{{Status: "failed"}}, rootStatus: "active", childCount: 1, want: "Worker failed"},
		{name: "active worker", queueStatus: "active", rows: []runChildRow{{Status: "active"}, {Status: "active"}}, rootStatus: "active", childCount: 2, want: "2 workers active"},
		{name: "active coordinator", queueStatus: "active", rows: nil, rootStatus: "active", childCount: 0, want: "Coordinator active"},
		{name: "pending workers", queueStatus: "pending", rows: []runChildRow{{Status: "pending"}}, rootStatus: "pending", childCount: 1, want: "1 worker queued"},
		{name: "completed children", queueStatus: "completed", rows: []runChildRow{{Status: "completed"}, {Status: "completed"}}, rootStatus: "completed", childCount: 2, want: "2 workers settled"},
		{name: "no children fallback", queueStatus: "completed", rows: nil, rootStatus: "completed", childCount: 0, want: "No delegated workers"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := summarizeRunBlocker(tc.queueStatus, tc.rows, tc.rootStatus, tc.childCount); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestBuildRunListClusters(t *testing.T) {
	previousNow := runListNow
	runListNow = func() time.Time {
		return time.Date(2026, time.March, 26, 8, 0, 0, 0, time.UTC)
	}
	t.Cleanup(func() {
		runListNow = previousNow
	})

	roots := []runListRow{
		{
			ID:           "run-root",
			Objective:    "Ship the landing page",
			AgentID:      "assistant",
			Status:       "active",
			QueueStatus:  "needs_approval",
			ModelLane:    "",
			ModelID:      "gpt-5.4",
			InputTokens:  34,
			OutputTokens: 55,
			CreatedAt:    "2026-03-25 10:00:00",
			UpdatedAt:    "2026-03-25 11:00:00",
		},
	}
	descendants := map[string][]runChildRow{
		"run-root": {
			{
				RootID:       "run-root",
				ID:           "run-child",
				Objective:    "Apply the patch",
				AgentID:      "patcher",
				Status:       "needs_approval",
				ModelLane:    "build",
				ModelID:      "gpt-5.4-mini",
				InputTokens:  8,
				OutputTokens: 13,
				CreatedAt:    "2026-03-25 10:10:00",
				UpdatedAt:    "2026-03-25 10:20:00",
				Depth:        1,
			},
		},
	}

	clusters := buildRunListClusters(roots, descendants)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	cluster := clusters[0]
	if cluster.Root.Status != "needs_approval" {
		t.Fatalf("expected root queue status needs_approval, got %q", cluster.Root.Status)
	}
	if cluster.Root.ModelDisplay != "gpt-5.4" {
		t.Fatalf("expected persisted model display, got %q", cluster.Root.ModelDisplay)
	}
	if cluster.Root.TokenSummary != "34 in / 55 out" {
		t.Fatalf("expected token summary, got %q", cluster.Root.TokenSummary)
	}
	if !strings.Contains(cluster.Root.StartedAtExact, "UTC") {
		t.Fatalf("expected exact UTC timestamp, got %q", cluster.Root.StartedAtExact)
	}
	if cluster.ChildCountLabel != "1 worker" {
		t.Fatalf("expected child count label, got %q", cluster.ChildCountLabel)
	}
	if cluster.BlockerLabel != "patcher waiting on approval" {
		t.Fatalf("expected blocker label, got %q", cluster.BlockerLabel)
	}
	if len(cluster.Children) != 1 || cluster.Children[0].Depth != 1 {
		t.Fatalf("expected one depth-1 child, got %#v", cluster.Children)
	}
}

func TestLoadRunDescendantsIncludesNestedWorkers(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	h.insertRunAt(t, "run-root", "conv-tree", "Coordinate the tree", "active", "2026-03-25 10:00:00")
	if _, err := h.db.RawDB().Exec(
		`INSERT INTO runs
		 (id, conversation_id, agent_id, parent_run_id, team_id, objective, workspace_root, status, created_at, updated_at)
		 VALUES
		 ('run-child', 'conv-tree', 'researcher', 'run-root', 'repo-task-team', 'Inspect OpenClaw', ?, 'completed', '2026-03-25 10:05:00', '2026-03-25 10:06:00'),
		 ('run-grandchild', 'conv-tree', 'reviewer', 'run-child', 'repo-task-team', 'Review notes', ?, 'completed', '2026-03-25 10:07:00', '2026-03-25 10:08:00')`,
		h.workspaceRoot,
		h.workspaceRoot,
	); err != nil {
		t.Fatalf("insert descendant runs: %v", err)
	}

	rows := []runListRow{{ID: "run-root"}}
	descendants, err := loadRunDescendants(t.Context(), h.db.RawDB(), rows)
	if err != nil {
		t.Fatalf("load descendants: %v", err)
	}

	got := descendants["run-root"]
	if len(got) != 2 {
		t.Fatalf("expected 2 descendants, got %#v", got)
	}
	if got[0].ID != "run-child" || got[0].Depth != 1 {
		t.Fatalf("expected first descendant to be direct child, got %#v", got[0])
	}
	if got[1].ID != "run-grandchild" || got[1].Depth != 2 {
		t.Fatalf("expected second descendant to be depth 2, got %#v", got[1])
	}
}
