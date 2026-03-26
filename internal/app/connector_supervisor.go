package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

type connectorHealthReporter interface {
	ConnectorHealthSnapshot() model.ConnectorHealthSnapshot
}

type connectorSupervisorConfig struct {
	StartupGrace       time.Duration
	CheckInterval      time.Duration
	PersistInterval    time.Duration
	RestartCooldown    time.Duration
	MaxRestartsPerHour int
	now                func() time.Time
	persistSnapshots   func(context.Context, []model.ConnectorHealthSnapshot) error
}

type connectorSupervisor struct {
	connectors       []model.Connector
	cfg              connectorSupervisorConfig
	lastPersistedAt  time.Time
	lastPersistedMap map[string]model.ConnectorHealthSnapshot
}

type supervisedConnector struct {
	connector         model.Connector
	startedAt         time.Time
	cancel            context.CancelFunc
	done              chan error
	running           bool
	restartRequested  bool
	nextStartAt       time.Time
	restartTimestamps []time.Time
}

func newConnectorSupervisor(connectors []model.Connector, cfg connectorSupervisorConfig) *connectorSupervisor {
	if cfg.StartupGrace <= 0 {
		cfg.StartupGrace = 5 * time.Second
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 250 * time.Millisecond
	}
	if cfg.PersistInterval <= 0 {
		cfg.PersistInterval = connectorHealthPersistInterval
	}
	if cfg.RestartCooldown <= 0 {
		cfg.RestartCooldown = 2 * time.Second
	}
	if cfg.MaxRestartsPerHour <= 0 {
		cfg.MaxRestartsPerHour = 3
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}

	return &connectorSupervisor{
		connectors:       connectors,
		cfg:              cfg,
		lastPersistedMap: make(map[string]model.ConnectorHealthSnapshot, len(connectors)),
	}
}

func (s *connectorSupervisor) Run(ctx context.Context) error {
	if len(s.connectors) == 0 {
		<-ctx.Done()
		return ctx.Err()
	}

	states := make([]*supervisedConnector, 0, len(s.connectors))
	var wg sync.WaitGroup
	now := s.cfg.now()
	for _, connector := range s.connectors {
		state := &supervisedConnector{connector: connector}
		s.startConnector(ctx, state, now, &wg)
		states = append(states, state)
	}
	s.persistSnapshots(ctx, now, true)

	ticker := time.NewTicker(s.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			for _, state := range states {
				if state.cancel != nil {
					state.cancel()
				}
			}
			wg.Wait()
			return ctx.Err()
		case <-ticker.C:
			now := s.cfg.now()
			for _, state := range states {
				s.collectExit(ctx, state, now)
				s.restartIfDegraded(state, now)
				s.startPending(ctx, state, now, &wg)
			}
			s.persistSnapshots(ctx, now, false)
		}
	}
}

func (s *connectorSupervisor) startConnector(ctx context.Context, state *supervisedConnector, now time.Time, wg *sync.WaitGroup) {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)

	state.startedAt = now
	state.cancel = cancel
	state.done = done
	state.running = true

	wg.Add(1)
	go func(connector model.Connector) {
		defer wg.Done()
		done <- connector.Start(runCtx)
	}(state.connector)
}

func (s *connectorSupervisor) collectExit(ctx context.Context, state *supervisedConnector, now time.Time) {
	if !state.running || state.done == nil {
		return
	}

	select {
	case err := <-state.done:
		state.running = false
		state.cancel = nil
		state.done = nil

		if ctx.Err() != nil {
			return
		}
		if state.restartRequested {
			return
		}
		if err == nil || (!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
			s.requestRestart(state, now)
		}
	default:
	}
}

func (s *connectorSupervisor) restartIfDegraded(state *supervisedConnector, now time.Time) {
	if !state.running || state.restartRequested || now.Sub(state.startedAt) < s.cfg.StartupGrace {
		return
	}

	reporter, ok := state.connector.(connectorHealthReporter)
	if !ok {
		return
	}
	snapshot := reporter.ConnectorHealthSnapshot()
	if snapshot.State != model.ConnectorHealthDegraded || !snapshot.RestartSuggested {
		return
	}
	if !s.requestRestart(state, now) {
		return
	}
	if state.cancel != nil {
		state.cancel()
	}
}

func (s *connectorSupervisor) startPending(ctx context.Context, state *supervisedConnector, now time.Time, wg *sync.WaitGroup) {
	if state.running || !state.restartRequested || now.Before(state.nextStartAt) || ctx.Err() != nil {
		return
	}

	state.restartRequested = false
	s.startConnector(ctx, state, now, wg)
}

func (s *connectorSupervisor) requestRestart(state *supervisedConnector, now time.Time) bool {
	state.restartTimestamps = pruneRestarts(state.restartTimestamps, now)
	if len(state.restartTimestamps) >= s.cfg.MaxRestartsPerHour {
		state.restartRequested = false
		return false
	}

	state.restartRequested = true
	state.nextStartAt = now.Add(s.cfg.RestartCooldown)
	state.restartTimestamps = append(state.restartTimestamps, now)
	return true
}

func pruneRestarts(restarts []time.Time, now time.Time) []time.Time {
	if len(restarts) == 0 {
		return restarts
	}

	cutoff := now.Add(-time.Hour)
	kept := restarts[:0]
	for _, restartAt := range restarts {
		if restartAt.After(cutoff) {
			kept = append(kept, restartAt)
		}
	}
	return kept
}

func (s *connectorSupervisor) persistSnapshots(ctx context.Context, now time.Time, force bool) {
	if s.cfg.persistSnapshots == nil {
		return
	}

	snapshots := collectConnectorHealthSnapshots(s.connectors)
	if len(snapshots) == 0 {
		return
	}
	if !force && !s.shouldPersistSnapshots(snapshots, now) {
		return
	}
	if err := s.cfg.persistSnapshots(ctx, snapshots); err != nil {
		if ctx.Err() == nil {
			fmt.Printf("connector health persist warning: %v\n", err)
		}
		return
	}
	s.lastPersistedAt = now
	s.lastPersistedMap = make(map[string]model.ConnectorHealthSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		s.lastPersistedMap[snapshot.ConnectorID] = snapshot
	}
}

func (s *connectorSupervisor) shouldPersistSnapshots(snapshots []model.ConnectorHealthSnapshot, now time.Time) bool {
	if s.lastPersistedAt.IsZero() || now.Sub(s.lastPersistedAt) >= s.cfg.PersistInterval {
		return true
	}
	if len(snapshots) != len(s.lastPersistedMap) {
		return true
	}
	for _, snapshot := range snapshots {
		prior, ok := s.lastPersistedMap[snapshot.ConnectorID]
		if !ok {
			return true
		}
		if prior.State != snapshot.State || prior.Summary != snapshot.Summary || prior.RestartSuggested != snapshot.RestartSuggested {
			return true
		}
	}
	return false
}
