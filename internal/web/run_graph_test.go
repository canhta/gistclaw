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
			{ID: "root", AgentID: "lead", BaseProfile: model.BaseProfileOperator, Status: model.RunStatusActive},
			{ID: "r1", ParentRunID: "root", AgentID: "researcher", BaseProfile: model.BaseProfileResearch, Status: model.RunStatusCompleted},
			{ID: "p1", ParentRunID: "root", AgentID: "patcher", BaseProfile: model.BaseProfileWrite, Status: model.RunStatusNeedsApproval},
			{ID: "v1", ParentRunID: "p1", AgentID: "verifier", BaseProfile: model.BaseProfileVerify, Status: model.RunStatusPending},
		},
	}

	view := buildRunGraphView(snapshot)

	if got := findGraphNode(t, view.Nodes, "root").Kind; got != "root" {
		t.Fatalf("expected root node kind, got %q", got)
	}
	if got := findGraphNode(t, view.Nodes, "root").LaneID; got != "coordination" {
		t.Fatalf("expected root node to use coordination lane, got %q", got)
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
	if got := findGraphEdge(t, view.Edges, "root", "r1", "delegates").Label; got != "research" {
		t.Fatalf("expected research edge label, got %q", got)
	}
	if !hasGraphEdge(view.Edges, "r1", "root", "reports") {
		t.Fatal("expected report-back edge for completed child")
	}
	if got := findGraphEdge(t, view.Edges, "r1", "root", "reports").Label; got != "report" {
		t.Fatalf("expected report edge label, got %q", got)
	}
	if !hasGraphEdge(view.Edges, "root", "p1", "blocked") {
		t.Fatal("expected blocked edge for approval wait")
	}
	if got := findGraphEdge(t, view.Edges, "root", "p1", "blocked").Label; got != "approve" {
		t.Fatalf("expected approve edge label, got %q", got)
	}
}

func TestBuildRunGraphViewUsesRootAgentForCoordinationLane(t *testing.T) {
	t.Parallel()

	snapshot := replay.RunGraphSnapshot{
		RootRunID: "root",
		Nodes: []replay.GraphNode{
			{ID: "root", AgentID: "lead", BaseProfile: model.BaseProfileOperator, Status: model.RunStatusActive},
			{ID: "child-front", ParentRunID: "root", AgentID: "lead", BaseProfile: model.BaseProfileOperator, Status: model.RunStatusCompleted},
		},
	}

	view := buildRunGraphView(snapshot)
	if got := findGraphNode(t, view.Nodes, "child-front").LaneID; got != "coordination" {
		t.Fatalf("expected renamed front agent child to use coordination lane, got %q", got)
	}
	if got := findGraphEdge(t, view.Edges, "root", "child-front", "delegates").Label; got != "coordinate" {
		t.Fatalf("expected renamed front agent edge label %q, got %q", "coordinate", got)
	}
}

func TestBuildRunGraphViewUsesBaseProfilesForCustomSpecialistIDs(t *testing.T) {
	t.Parallel()

	snapshot := replay.RunGraphSnapshot{
		RootRunID: "root",
		Nodes: []replay.GraphNode{
			{ID: "root", AgentID: "lead", BaseProfile: model.BaseProfileOperator, Status: model.RunStatusActive},
			{ID: "writer-1", ParentRunID: "root", AgentID: "writer", BaseProfile: model.BaseProfileWrite, Status: model.RunStatusCompleted},
			{ID: "qa-1", ParentRunID: "root", AgentID: "qa", BaseProfile: model.BaseProfileReview, Status: model.RunStatusCompleted},
		},
	}

	view := buildRunGraphView(snapshot)

	if got := findGraphNode(t, view.Nodes, "writer-1").LaneID; got != "build" {
		t.Fatalf("expected write specialist lane %q, got %q", "build", got)
	}
	if got := findGraphEdge(t, view.Edges, "root", "writer-1", "delegates").Label; got != "build" {
		t.Fatalf("expected write specialist edge label %q, got %q", "build", got)
	}
	if got := findGraphNode(t, view.Nodes, "qa-1").Kind; got != "review" {
		t.Fatalf("expected review specialist kind %q, got %q", "review", got)
	}
	if got := findGraphNode(t, view.Nodes, "qa-1").LaneID; got != "review" {
		t.Fatalf("expected review specialist lane %q, got %q", "review", got)
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

func findGraphEdge(t *testing.T, edges []runGraphEdgeView, from, to, kind string) runGraphEdgeView {
	t.Helper()

	for _, edge := range edges {
		if edge.From == from && edge.To == to && edge.Kind == kind {
			return edge
		}
	}
	t.Fatalf("edge %q -> %q (%s) not found", from, to, kind)
	return runGraphEdgeView{}
}
