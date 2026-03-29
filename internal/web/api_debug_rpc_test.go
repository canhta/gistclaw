package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canhta/gistclaw/internal/debugrpc"
)

type stubDebugRPCSource struct {
	status      debugrpc.Status
	err         error
	calledProbe string
}

func (s *stubDebugRPCSource) DebugRPCStatus(_ context.Context, probe string) (debugrpc.Status, error) {
	s.calledProbe = probe
	if s.err != nil {
		return debugrpc.Status{}, s.err
	}
	return s.status, nil
}

func TestDebugRPCStatusReturnsSelectedProbeSnapshot(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	source := &stubDebugRPCSource{
		status: debugrpc.Status{
			Summary: debugrpc.Summary{
				ProbeCount:    4,
				ReadOnly:      true,
				DefaultProbe:  "status",
				SelectedProbe: "connector_health",
			},
			Probes: []debugrpc.Probe{
				{
					Name:        "status",
					Label:       "Status",
					Description: "Inspect active runs, approvals, and storage health.",
				},
				{
					Name:        "connector_health",
					Label:       "Connector health",
					Description: "Inspect configured connector health snapshots.",
				},
			},
			Result: debugrpc.Result{
				Probe:           "connector_health",
				Label:           "Connector health",
				Summary:         "2 connector snapshots loaded",
				ExecutedAt:      "2026-03-29T10:10:00Z",
				ExecutedAtLabel: "2026-03-29 10:10:00 UTC",
				Data: map[string]any{
					"summary": map[string]any{
						"total": 2,
					},
				},
			},
		},
	}
	h.rawServer.debugRPC = source

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/debug/rpc?probe=connector_health", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Summary struct {
			ProbeCount    int    `json:"probe_count"`
			ReadOnly      bool   `json:"read_only"`
			DefaultProbe  string `json:"default_probe"`
			SelectedProbe string `json:"selected_probe"`
		} `json:"summary"`
		Probes []struct {
			Name  string `json:"name"`
			Label string `json:"label"`
		} `json:"probes"`
		Result struct {
			Probe   string         `json:"probe"`
			Summary string         `json:"summary"`
			Data    map[string]any `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode debug rpc response: %v", err)
	}

	if resp.Summary.ProbeCount != 4 || !resp.Summary.ReadOnly || resp.Summary.SelectedProbe != "connector_health" {
		t.Fatalf("unexpected summary: %+v", resp.Summary)
	}
	if len(resp.Probes) != 2 || resp.Probes[1].Name != "connector_health" {
		t.Fatalf("unexpected probes: %+v", resp.Probes)
	}
	if resp.Result.Probe != "connector_health" || resp.Result.Summary != "2 connector snapshots loaded" {
		t.Fatalf("unexpected result: %+v", resp.Result)
	}
	if got := resp.Result.Data["summary"]; got == nil {
		t.Fatalf("expected summary payload, got %+v", resp.Result.Data)
	}
	if source.calledProbe != "connector_health" {
		t.Fatalf("expected probe passthrough, got %q", source.calledProbe)
	}
}

func TestDebugRPCStatusRejectsUnknownProbe(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	h.rawServer.debugRPC = &stubDebugRPCSource{err: debugrpc.ErrUnknownProbe}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/debug/rpc?probe=drop_database", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if !errors.Is(debugrpc.ErrUnknownProbe, debugrpc.ErrUnknownProbe) {
		t.Fatal("expected sentinel error to remain comparable")
	}
	if resp["message"] == "" {
		t.Fatalf("expected error message, got %+v", resp)
	}
}
