package web

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

func TestFormatRunTokenSummary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		inputTokens  int
		outputTokens int
		want         string
	}{
		{name: "small values stay raw", inputTokens: 34, outputTokens: 55, want: "34 in / 55 out"},
		{name: "thousands compact to k", inputTokens: 2730, outputTokens: 28, want: "2.7K in / 28 out"},
		{name: "millions compact to m", inputTokens: 1200000, outputTokens: 84000, want: "1.2M in / 84K out"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := formatRunTokenSummary(tc.inputTokens, tc.outputTokens); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestFormatRunCompactTimestamp(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, time.March, 25, 10, 15, 9, 0, time.UTC)
	if got := formatRunCompactTimestamp(ts); got != "2026-03-25 10:15 UTC" {
		t.Fatalf("expected compact timestamp, got %q", got)
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
		{name: "failure without agent", queueStatus: "failed", rows: []runChildRow{{Status: "failed"}}, rootStatus: "active", childCount: 1, want: "Sub-agent failed"},
		{name: "active worker", queueStatus: "active", rows: []runChildRow{{Status: "active"}, {Status: "active"}}, rootStatus: "active", childCount: 2, want: "2 sub-agents active"},
		{name: "active coordinator", queueStatus: "active", rows: nil, rootStatus: "active", childCount: 0, want: "Lead agent active"},
		{name: "pending workers", queueStatus: "pending", rows: []runChildRow{{Status: "pending"}}, rootStatus: "pending", childCount: 1, want: "1 sub-agent queued"},
		{name: "completed children", queueStatus: "completed", rows: []runChildRow{{Status: "completed"}, {Status: "completed"}}, rootStatus: "completed", childCount: 2, want: "2 sub-agents settled"},
		{name: "no children fallback", queueStatus: "completed", rows: nil, rootStatus: "completed", childCount: 0, want: "No sub-agents"},
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
	if cluster.Root.StartedAtShort != "2026-03-25 10:00 UTC" {
		t.Fatalf("expected compact started-at label, got %q", cluster.Root.StartedAtShort)
	}
	if cluster.Root.LastActivityShort != "2026-03-25 11:00 UTC" {
		t.Fatalf("expected compact last-activity label, got %q", cluster.Root.LastActivityShort)
	}
	if cluster.Root.StartedAtExact != "2026-03-25 10:00:00 UTC" {
		t.Fatalf("expected exact started-at timestamp for drill-down, got %q", cluster.Root.StartedAtExact)
	}
	if cluster.ChildCountLabel != "1 sub-agent" {
		t.Fatalf("expected child count label, got %q", cluster.ChildCountLabel)
	}
	if cluster.BlockerLabel != "patcher waiting on approval" {
		t.Fatalf("expected blocker label, got %q", cluster.BlockerLabel)
	}
	if len(cluster.Children) != 1 || cluster.Children[0].Depth != 1 {
		t.Fatalf("expected one depth-1 child, got %#v", cluster.Children)
	}
}

func TestBuildRunQueueStripUsesTaskFramedHeadlines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		clusters []runListClusterView
		want     string
	}{
		{
			name: "empty",
			want: "No active work yet. Start a task to see progress here.",
		},
		{
			name: "needs approval",
			clusters: []runListClusterView{{
				Root: runListItem{Status: "needs_approval"},
			}},
			want: "Some work is waiting on you.",
		},
		{
			name: "active",
			clusters: []runListClusterView{{
				Root: runListItem{Status: "active"},
				Children: []runListItem{
					{Status: "completed"},
				},
			}},
			want: "See what is running, waiting on you, or done.",
		},
		{
			name: "settled",
			clusters: []runListClusterView{{
				Root: runListItem{Status: "completed"},
			}},
			want: "Recent work is settled.",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := buildRunQueueStrip(tc.clusters).Headline; got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestRunGraphHeadlineUsesTaskLanguage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		summary runGraphSummaryView
		want    string
	}{
		{
			name:    "approval",
			summary: runGraphSummaryView{NeedsApproval: 1},
			want:    "1 task waiting on you.",
		},
		{
			name:    "active",
			summary: runGraphSummaryView{Active: 2},
			want:    "2 tasks in progress.",
		},
		{
			name:    "pending",
			summary: runGraphSummaryView{Pending: 1},
			want:    "1 task queued to start.",
		},
		{
			name:    "failed",
			summary: runGraphSummaryView{Failed: 1},
			want:    "1 task failed on this map.",
		},
		{
			name:    "interrupted",
			summary: runGraphSummaryView{Interrupted: 2},
			want:    "2 tasks interrupted.",
		},
		{
			name:    "completed",
			summary: runGraphSummaryView{Completed: 3, Total: 3},
			want:    "Everything on this map is done.",
		},
		{
			name:    "fallback",
			summary: runGraphSummaryView{},
			want:    "Status available.",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := runGraphHeadline(tc.summary); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestFormatStructuredTextView(t *testing.T) {
	t.Parallel()

	view := buildStructuredTextView("OpenClaw summary\n\n1. Research the system\n2. Build the page\n3. Verify the output", 3)
	if got := len(view.Blocks); got != 2 {
		t.Fatalf("expected 2 blocks, got %d", got)
	}
	if view.Blocks[0].Kind != "paragraph" || view.Blocks[0].Text != "OpenClaw summary" {
		t.Fatalf("unexpected first block: %+v", view.Blocks[0])
	}
	if view.Blocks[1].Kind != "ordered_list" || len(view.Blocks[1].Items) != 3 {
		t.Fatalf("expected ordered list block, got %+v", view.Blocks[1])
	}
	if view.PreviewText != "OpenClaw summary\n1. Research the system\n2. Build the page" {
		t.Fatalf("unexpected preview text %q", view.PreviewText)
	}
	if !view.HasOverflow {
		t.Fatal("expected preview to report overflow")
	}
}

func TestBuildStructuredTextViewRendersMarkdownHTML(t *testing.T) {
	t.Parallel()

	view := buildStructuredTextView("## OpenClaw\n\n- Research\n- Build\n\n<script>alert('x')</script>", 3)
	if !strings.Contains(string(view.HTML), "<h2>OpenClaw</h2>") {
		t.Fatalf("expected markdown heading HTML, got %q", view.HTML)
	}
	if !strings.Contains(string(view.HTML), "<ul>") {
		t.Fatalf("expected markdown list HTML, got %q", view.HTML)
	}
	if strings.Contains(string(view.HTML), "<script>") {
		t.Fatalf("expected markdown HTML to be sanitized, got %q", view.HTML)
	}
}

func TestRenderTerminalLogHTMLSanitizesANSIOutput(t *testing.T) {
	t.Parallel()

	got := string(renderTerminalLogHTML("\x1b[31mFAIL\x1b[0m<script>alert('x')</script>"))
	if !strings.Contains(got, "FAIL") {
		t.Fatalf("expected rendered terminal HTML to contain text, got %q", got)
	}
	if strings.Contains(got, "<script>") {
		t.Fatalf("expected rendered terminal HTML to be sanitized, got %q", got)
	}
}

func TestLiveToolLogStateAggregatesTerminalHTML(t *testing.T) {
	t.Parallel()

	historyPayload, err := json.Marshal(runToolLogPayload{
		ToolCallID: "call-coder",
		ToolName:   "coder_exec",
		Stream:     "terminal",
		Text:       "\x1b[31mFA",
	})
	if err != nil {
		t.Fatalf("marshal history payload: %v", err)
	}
	state := newLiveToolLogState([]model.Event{{
		Kind:        "tool_log_recorded",
		PayloadJSON: historyPayload,
		CreatedAt:   time.Date(2026, time.March, 26, 5, 0, 0, 0, time.UTC),
	}})

	got := state.Apply(runToolLogPayload{
		ToolCallID: "call-coder",
		ToolName:   "coder_exec",
		Stream:     "terminal",
		Text:       "IL\x1b[0m",
	})

	if got.EntryKey != "call-coder::terminal" {
		t.Fatalf("expected stable entry key, got %+v", got)
	}
	if got.Body != "\x1b[31mFAIL\x1b[0m" {
		t.Fatalf("expected aggregated terminal body, got %q", got.Body)
	}
	if !strings.Contains(string(got.BodyHTML), "FAIL") {
		t.Fatalf("expected rendered terminal html, got %q", got.BodyHTML)
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
