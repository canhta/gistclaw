package web

import (
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/replay"
)

func TestBuildRunGraphViewAssignsLanesKindsAndEdgeSemantics(t *testing.T) {
	t.Parallel()

	snapshot := replay.RunGraphSnapshot{
		RootRunID: "root",
		Nodes: []replay.GraphNode{
			{ID: "root", AgentID: "assistant", Status: model.RunStatusActive},
			{ID: "r1", ParentRunID: "root", AgentID: "researcher", Status: model.RunStatusCompleted},
			{ID: "p1", ParentRunID: "root", AgentID: "patcher", Status: model.RunStatusNeedsApproval},
			{ID: "v1", ParentRunID: "p1", AgentID: "verifier", Status: model.RunStatusPending},
		},
	}

	view := buildRunGraphView(snapshot)

	if got := findGraphNode(t, view.Nodes, "root").Kind; got != "root" {
		t.Fatalf("expected root node kind, got %q", got)
	}
	if got := findGraphNode(t, view.Nodes, "r1").LaneID; got != "research" {
		t.Fatalf("expected research lane, got %q", got)
	}
	if got := findGraphNode(t, view.Nodes, "v1").Kind; got != "verify" {
		t.Fatalf("expected verify node kind, got %q", got)
	}
	if !hasGraphEdge(view.Edges, "root", "r1", "delegates") {
		t.Fatal("expected delegates edge")
	}
	if !hasGraphEdge(view.Edges, "r1", "root", "reports") {
		t.Fatal("expected report-back edge for completed child")
	}
	if !hasGraphEdge(view.Edges, "root", "p1", "blocked") {
		t.Fatal("expected blocked edge for approval wait")
	}
}

func findGraphNode(t *testing.T, nodes []runGraphNodeView, id string) runGraphNodeView {
	t.Helper()

	for _, node := range nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("node %q not found", id)
	return runGraphNodeView{}
}

func hasGraphEdge(edges []runGraphEdgeView, from, to, kind string) bool {
	for _, edge := range edges {
		if edge.From == from && edge.To == to && edge.Kind == kind {
			return true
		}
	}
	return false
}
