package app

import (
	"context"
	"fmt"
)

func (a *App) Start(ctx context.Context) error {
	if a.runtime != nil {
		if _, err := a.runtime.ReconcileInterrupted(ctx); err != nil {
			return fmt.Errorf("reconcile interrupted: %w", err)
		}
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
