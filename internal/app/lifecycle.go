package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

func (a *App) Prepare(ctx context.Context) error {
	a.prepareMu.Lock()
	defer a.prepareMu.Unlock()

	if a.prepared {
		return nil
	}

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
			log.Printf("runtime warn expire_stale_approvals_failed err=%v", err)
		} else {
			expiredApprovals = n
		}
	}
	if a.scheduler != nil {
		if err := a.scheduler.Repair(ctx); err != nil {
			return fmt.Errorf("repair scheduler: %w", err)
		}
	}

	// Advisory disk-space check.
	checkDiskSpace(a.cfg.DatabasePath)

	log.Printf(
		"runtime info startup_reconciled reconciled_runs=%d expired_approvals=%d",
		reconciledRuns,
		expiredApprovals,
	)

	// Drain any pending outbound intents from a previous session for all connectors.
	for _, c := range a.connectors {
		if err := c.Drain(ctx); err != nil {
			// Drain failure is non-fatal — log and continue.
			log.Printf("runtime warn connector_drain connector=%s err=%v", c.Metadata().ID, err)
		}
	}

	a.prepared = true
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
		log.Printf("runtime warn low_disk_space available_bytes=%d", available)
	}
}

func (a *App) Start(ctx context.Context) error {
	if err := a.Prepare(ctx); err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if a.runtime != nil {
		a.runtime.SetAsyncContext(runCtx)
	}
	a.supervisor = nil

	serviceCount := len(a.connectors)
	errCh := make(chan error, len(a.connectors)+1)
	var wg sync.WaitGroup
	var webHTTP *http.Server

	if a.webServer != nil && a.cfg.Web.ListenAddr != "" {
		listener, err := net.Listen("tcp", a.cfg.Web.ListenAddr)
		if err != nil {
			return fmt.Errorf("web listen: %w", err)
		}
		a.setWebAddress(listener.Addr().String())
		webHTTP = &http.Server{Handler: a.webServer}
		serviceCount++
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := webHTTP.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				select {
				case errCh <- fmt.Errorf("web server: %w", err):
				default:
				}
				cancel()
			}
		}()
	} else {
		a.setWebAddress("")
	}

	if a.scheduler != nil {
		serviceCount++
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.scheduler.Start(runCtx); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
				select {
				case errCh <- fmt.Errorf("scheduler: %w", err):
				default:
				}
				cancel()
			}
		}()
	}

	if serviceCount == 0 {
		<-ctx.Done()
		return ctx.Err()
	}

	if len(a.connectors) > 0 {
		supervisorConfig := a.supervisorConfig
		supervisorConfig.persistSnapshots = func(ctx context.Context, snapshots []model.ConnectorHealthSnapshot) error {
			return storeConnectorHealthSnapshots(ctx, a.db, snapshots)
		}
		supervisor := newConnectorSupervisor(a.connectors, supervisorConfig)
		a.supervisor = supervisor
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := supervisor.Run(runCtx); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
				select {
				case errCh <- fmt.Errorf("connector supervisor: %w", err):
				default:
				}
				cancel()
			}
		}()
	}

	shutdownWeb := func() {
		if webHTTP == nil {
			return
		}
		shutdownCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = webHTTP.Shutdown(shutdownCtx)
	}

	select {
	case err := <-errCh:
		cancel()
		shutdownWeb()
		wg.Wait()
		if a.runtime != nil {
			a.runtime.WaitAsync()
		}
		return err
	case <-ctx.Done():
		cancel()
		shutdownWeb()
		wg.Wait()
		if a.runtime != nil {
			a.runtime.WaitAsync()
		}
		return ctx.Err()
	}
}

func (a *App) Stop() error {
	if a.toolCloser != nil {
		if err := a.toolCloser.Close(); err != nil {
			return err
		}
	}
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}
