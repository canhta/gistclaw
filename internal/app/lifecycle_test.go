package app

import (
	"context"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type stubConnector struct {
	id      string
	started chan struct{}
}

func (c *stubConnector) ID() string { return c.id }

func (c *stubConnector) Start(ctx context.Context) error {
	select {
	case <-c.started:
	default:
		close(c.started)
	}
	<-ctx.Done()
	return ctx.Err()
}

func (c *stubConnector) Notify(context.Context, string, model.ReplayDelta, string) error { return nil }

func (c *stubConnector) Drain(context.Context) error { return nil }

func TestLifecycle_StartsAndStopsCleanly(t *testing.T) {
	cfg := Config{
		DatabasePath:  ":memory:",
		StateDir:      t.TempDir(),
		WorkspaceRoot: t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
	}
	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Start returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5 seconds after cancel")
	}

	if err := app.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

func TestLifecycle_SIGINTTriggersShutdown(t *testing.T) {
	cfg := Config{
		DatabasePath:  ":memory:",
		StateDir:      t.TempDir(),
		WorkspaceRoot: t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
	}
	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Start returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5 seconds after shutdown signal")
	}
}

func TestLifecycle_StartRunsWiredConnectors(t *testing.T) {
	cfg := Config{
		DatabasePath:  ":memory:",
		StateDir:      t.TempDir(),
		WorkspaceRoot: t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
	}
	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	connector := &stubConnector{id: "stub", started: make(chan struct{})}
	app.connectors = []model.Connector{connector}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Start(ctx)
	}()

	select {
	case <-connector.started:
		cancel()
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected connector Start to be called")
	}

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Start returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}
