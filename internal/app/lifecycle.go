package app

import (
	"context"
	"fmt"
	"os"
	"syscall"
)

func (a *App) Prepare(ctx context.Context) error {
	if err := ensureAdminToken(a.db); err != nil {
		return err
	}

	reconciledRuns := 0
	expiredApprovals := 0

	if a.runtime != nil {
		report, err := a.runtime.ReconcileInterrupted(ctx)
		if err != nil {
			return fmt.Errorf("reconcile interrupted: %w", err)
		}
		reconciledRuns = report.ReconciledCount

		n, err := a.runtime.ExpireStaleApprovals(ctx)
		if err != nil {
			// Non-fatal — log and continue.
			fmt.Printf(`{"level":"warn","event":"expire_stale_approvals_failed","error":%q}`+"\n", err.Error())
		} else {
			expiredApprovals = n
		}
	}

	// Advisory disk-space check.
	checkDiskSpace(a.cfg.DatabasePath)

	fmt.Printf(`{"level":"info","event":"startup_reconciled","reconciled_runs":%d,"expired_approvals":%d}`+"\n",
		reconciledRuns, expiredApprovals)

	// Drain any pending outbound intents from a previous session for all connectors.
	for _, c := range a.connectors {
		if err := c.Drain(ctx); err != nil {
			// Drain failure is non-fatal — log and continue.
			fmt.Printf("connector %s: drain warning: %v\n", c.ID(), err)
		}
	}

	return nil
}

// checkDiskSpace emits a structured warning if available disk space on the
// database filesystem is below 500 MB. Advisory only — never returns an error.
func checkDiskSpace(dbPath string) {
	const lowThresholdBytes = 500 * 1024 * 1024 // 500 MB
	dir := dbPath
	if dir == "" || dir == ":memory:" {
		return
	}
	// Use the directory containing the database file.
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		// Try parent directory.
		for i := len(dir) - 1; i >= 0; i-- {
			if dir[i] == '/' || dir[i] == os.PathSeparator {
				dir = dir[:i]
				break
			}
		}
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return
	}
	available := stat.Bavail * uint64(stat.Bsize)
	if available < lowThresholdBytes {
		fmt.Printf(`{"level":"warn","event":"low_disk_space","available_bytes":%d}`+"\n", available)
	}
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
