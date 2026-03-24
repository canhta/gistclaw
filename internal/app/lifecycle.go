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

	return nil
}

func (a *App) Start(ctx context.Context) error {
	if err := a.Prepare(ctx); err != nil {
		return err
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

