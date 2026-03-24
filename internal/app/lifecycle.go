package app

import (
	"context"
	"fmt"
)

func (a *App) Prepare(ctx context.Context) error {
	if err := ensureAdminToken(a.db); err != nil {
		return err
	}

	if a.runtime != nil {
		if _, err := a.runtime.ReconcileInterrupted(ctx); err != nil {
			return fmt.Errorf("reconcile interrupted: %w", err)
		}
	}

	// Drain any pending outbound intents from a previous session for all connectors.
	for _, c := range a.connectors {
		if err := c.Drain(ctx); err != nil {
			// Drain failure is non-fatal — log and continue.
			fmt.Printf("connector %s: drain warning: %v\n", c.ID(), err)
		}
	}

	return nil
}

func (a *App) Start(ctx context.Context) error {
	if err := a.Prepare(ctx); err != nil {
		return err
	}

	// Start the cron scheduler in a background goroutine. It exits when ctx
	// is cancelled; any errors are non-fatal (individual schedule failures are
	// logged inline by the dispatcher).
	if a.scheduler != nil {
		go func() {
			if err := a.scheduler.Run(ctx); err != nil && ctx.Err() == nil {
				fmt.Printf("scheduler: exited unexpectedly: %v\n", err)
			}
		}()
	}

	<-ctx.Done()
	return ctx.Err()
}

func (a *App) Stop() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}
