package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type stubSupervisedConnector struct {
	id string

	startCount atomic.Int32
	started    chan struct{}

	mu        sync.Mutex
	startErrs []error
	health    model.ConnectorHealthSnapshot
}

func newStubSupervisedConnector(id string, startErrs ...error) *stubSupervisedConnector {
	return &stubSupervisedConnector{
		id:        id,
		started:   make(chan struct{}, 16),
		startErrs: append([]error(nil), startErrs...),
		health: model.ConnectorHealthSnapshot{
			ConnectorID:      id,
			State:            model.ConnectorHealthHealthy,
			Summary:          "healthy",
			RestartSuggested: false,
		},
	}
}

func (c *stubSupervisedConnector) Metadata() model.ConnectorMetadata {
	return model.NormalizeConnectorMetadata(model.ConnectorMetadata{ID: c.id})
}

func (c *stubSupervisedConnector) Start(ctx context.Context) error {
	c.startCount.Add(1)
	select {
	case c.started <- struct{}{}:
	default:
	}

	c.mu.Lock()
	var err error
	if len(c.startErrs) > 0 {
		err = c.startErrs[0]
		c.startErrs = c.startErrs[1:]
	}
	c.mu.Unlock()
	if err != nil {
		return err
	}

	<-ctx.Done()
	return ctx.Err()
}

func (c *stubSupervisedConnector) Notify(context.Context, string, model.ReplayDelta, string) error {
	return nil
}

func (c *stubSupervisedConnector) Drain(context.Context) error {
	return nil
}

func (c *stubSupervisedConnector) ConnectorHealthSnapshot() model.ConnectorHealthSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.health
}

func (c *stubSupervisedConnector) setHealth(state model.ConnectorHealthState, summary string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health.State = state
	c.health.Summary = summary
	c.health.RestartSuggested = state == model.ConnectorHealthDegraded
}

func (c *stubSupervisedConnector) waitForStarts(t *testing.T, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for i := 0; i < want; i++ {
		select {
		case <-c.started:
		case <-deadline:
			t.Fatalf("expected %d starts, got %d", want, c.startCount.Load())
		}
	}
}

func TestConnectorSupervisor_RestartsConnectorAfterStartErrorWithinBudget(t *testing.T) {
	connector := newStubSupervisedConnector("telegram", errors.New("boom"))
	supervisor := newConnectorSupervisor([]model.Connector{connector}, connectorSupervisorConfig{
		CheckInterval:      time.Millisecond,
		RestartCooldown:    time.Millisecond,
		MaxRestartsPerHour: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- supervisor.Run(ctx)
	}()

	connector.waitForStarts(t, 2, 200*time.Millisecond)
	cancel()

	if err := <-errCh; err != context.Canceled {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestConnectorSupervisor_RestartsDegradedConnectorWithinBudget(t *testing.T) {
	connector := newStubSupervisedConnector("telegram")
	supervisor := newConnectorSupervisor([]model.Connector{connector}, connectorSupervisorConfig{
		CheckInterval:      time.Millisecond,
		StartupGrace:       time.Nanosecond,
		RestartCooldown:    time.Millisecond,
		MaxRestartsPerHour: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- supervisor.Run(ctx)
	}()

	connector.waitForStarts(t, 1, 200*time.Millisecond)
	connector.setHealth(model.ConnectorHealthDegraded, "stale poll loop")
	connector.waitForStarts(t, 1, 200*time.Millisecond)
	cancel()

	if err := <-errCh; err != context.Canceled {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestConnectorSupervisor_StopsRestartingWhenBudgetIsExhausted(t *testing.T) {
	connector := newStubSupervisedConnector("telegram", errors.New("boom"), errors.New("boom"), errors.New("boom"))
	supervisor := newConnectorSupervisor([]model.Connector{connector}, connectorSupervisorConfig{
		CheckInterval:      time.Millisecond,
		RestartCooldown:    time.Millisecond,
		MaxRestartsPerHour: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- supervisor.Run(ctx)
	}()

	connector.waitForStarts(t, 2, 200*time.Millisecond)
	time.Sleep(25 * time.Millisecond)
	cancel()

	if got := connector.startCount.Load(); got != 2 {
		t.Fatalf("expected 2 starts with exhausted budget, got %d", got)
	}
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
