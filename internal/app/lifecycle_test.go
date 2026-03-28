package app

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type stubConnector struct {
	id      string
	started chan struct{}
	drains  atomic.Int32
}

func (c *stubConnector) Metadata() model.ConnectorMetadata {
	return model.NormalizeConnectorMetadata(model.ConnectorMetadata{ID: c.id})
}

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

func (c *stubConnector) Drain(context.Context) error {
	c.drains.Add(1)
	return nil
}

func TestLifecycle_StartsAndStopsCleanly(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
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
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
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
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
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

func TestLifecycle_StartServesWebUI(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
		Web: WebConfig{
			ListenAddr: "127.0.0.1:0",
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

	var addr string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		addr = app.WebAddress()
		if addr != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == "" {
		t.Fatal("expected web listener address to be published")
	}

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get("http://" + addr + "/work")
	if err != nil {
		t.Fatalf("GET /work failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 from /work while onboarding is pending, got %d", resp.StatusCode)
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Start returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestLifecycle_StartDoesNotRepeatPrepareSideEffects(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
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

	if err := app.Prepare(context.Background()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

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

	if got := connector.drains.Load(); got != 1 {
		t.Fatalf("expected connector drain to run once, got %d", got)
	}
}

func TestLifecycle_StartRecoversConnectorFailureWithinBudget(t *testing.T) {
	cfg := Config{
		DatabasePath: ":memory:",
		StateDir:     t.TempDir(),
		StorageRoot:  t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
	}
	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	connector := newStubSupervisedConnector("stub", errors.New("boom"))
	app.connectors = []model.Connector{connector}
	app.supervisorConfig = connectorSupervisorConfig{
		CheckInterval:      time.Millisecond,
		RestartCooldown:    time.Millisecond,
		MaxRestartsPerHour: 1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Start(ctx)
	}()

	connector.waitForStarts(t, 2, 300*time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Start returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestLifecycle_StartPersistsConnectorHealthSnapshots(t *testing.T) {
	cfg := Config{
		DatabasePath: filepath.Join(t.TempDir(), "state", "runtime.db"),
		StateDir:     filepath.Join(t.TempDir(), "state"),
		StorageRoot:  t.TempDir(),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
	}
	app, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	connector := newStubSupervisedConnector("telegram")
	connector.mu.Lock()
	connector.health.CheckedAt = time.Now().UTC()
	connector.mu.Unlock()
	app.connectors = []model.Connector{connector}
	app.supervisorConfig = connectorSupervisorConfig{
		CheckInterval: time.Millisecond,
		StartupGrace:  time.Hour,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Start(ctx)
	}()

	connector.waitForStarts(t, 1, 300*time.Millisecond)

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		var raw string
		err := app.db.RawDB().QueryRow("SELECT value FROM settings WHERE key = 'connector_health.telegram'").Scan(&raw)
		if err == nil && raw != "" {
			cancel()
			select {
			case err := <-errCh:
				if err != nil && err != context.Canceled {
					t.Fatalf("Start returned unexpected error: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Start did not return after cancel")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Start returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
	t.Fatal("expected connector health snapshot to be persisted")
}
