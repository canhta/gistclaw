package app

import (
	"context"
	"errors"
	"testing"

	"github.com/canhta/gistclaw/internal/debugrpc"
)

func TestApp_DebugRPCStatusReturnsCatalogAndSelectedProbe(t *testing.T) {
	application := setupCommandApp(t)

	status, err := application.DebugRPCStatus(context.Background(), "connector_health")
	if err != nil {
		t.Fatalf("DebugRPCStatus: %v", err)
	}

	if status.Summary.ProbeCount != 4 || !status.Summary.ReadOnly {
		t.Fatalf("unexpected summary: %+v", status.Summary)
	}
	if status.Summary.DefaultProbe != "status" || status.Summary.SelectedProbe != "connector_health" {
		t.Fatalf("unexpected selected probe summary: %+v", status.Summary)
	}
	if len(status.Probes) != 4 {
		t.Fatalf("expected 4 probes, got %d", len(status.Probes))
	}
	if status.Probes[0].Name != "status" || status.Probes[1].Name != "connector_health" {
		t.Fatalf("unexpected probe catalog: %+v", status.Probes)
	}
	if status.Result.Probe != "connector_health" || status.Result.Label != "Connector health" {
		t.Fatalf("unexpected probe result: %+v", status.Result)
	}
	if status.Result.ExecutedAt == "" || status.Result.ExecutedAtLabel == "" {
		t.Fatalf("expected execution timestamps, got %+v", status.Result)
	}
	summary, ok := status.Result.Data["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected summary map in result data, got %#v", status.Result.Data["summary"])
	}
	if got := summary["total"]; got != 0 {
		t.Fatalf("expected total=0, got %#v", got)
	}
}

func TestApp_DebugRPCStatusRejectsUnknownProbe(t *testing.T) {
	application := setupCommandApp(t)

	_, err := application.DebugRPCStatus(context.Background(), "drop_database")
	if !errors.Is(err, debugrpc.ErrUnknownProbe) {
		t.Fatalf("expected unknown probe error, got %v", err)
	}
}
