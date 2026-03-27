package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/canhta/gistclaw/internal/app"
	"github.com/canhta/gistclaw/internal/store"
)

const usage = `Usage: gistclaw <subcommand> [options]

Subcommands:
  serve      Start the GistClaw daemon
  run        Submit a task directly
  auth       Manage built-in browser access
  version    Print release/build metadata
  inspect    Inspect daemon state
  security   Run deployment security audit
  doctor     Run health checks (config, database, provider, workspace, storage, scheduler)
  backup     Back up the SQLite database to a timestamped .db.bak file
  export     Export runs, receipts, and approvals to a JSON file
  schedule   Manage scheduled tasks

Inspect subcommands:
  inspect status           Show active runs, interrupted runs, approvals, and storage health
  inspect runs             List all runs with status
  inspect replay <run_id>  Print replay for a run
  inspect config-paths     Print installer-owned directories for a config file
  inspect systemd-unit     Print the canonical systemd service unit
  inspect token            Print admin token from settings table

Flags:
  -h, --help         Show this help message
  -c, --config PATH  Use an explicit config file
`

func main() {
	os.Exit(runWithInput(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithInput(args, os.Stdin, stdout, stderr)
}

func runWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}

	configPath, args, err := parseConfigPath(args)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "auth":
		return runAuth(configPath, args[1:], stdin, stdout, stderr)
	case "serve":
		return runServe(configPath, stdout, stderr)
	case "run":
		return runTask(configPath, args[1:], stdout, stderr)
	case "version":
		return runVersion(stdout, stderr)
	case "inspect":
		return runInspect(configPath, args[1:], stdout, stderr)
	case "security":
		return runSecurity(configPath, args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(configPath, stdout, stderr)
	case "backup":
		return runBackup(args[1:], stdout, stderr)
	case "export":
		return runExport(args[1:], stdout, stderr)
	case "schedule":
		return runSchedule(configPath, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n%s", args[0], usage)
		return 1
	}
}

func runServe(configPath string, stdout, stderr io.Writer) int {
	application, err := loadPreparedApp(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
		return 1
	}
	defer func() { _ = application.Stop() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintln(stdout, "gistclaw serve: listening")
	if err := application.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(stderr, "serve failed: %v\n", err)
		return 1
	}
	return 0
}

func runTask(configPath string, args []string, stdout, stderr io.Writer) int {
	objective := strings.TrimSpace(strings.Join(args, " "))
	if objective == "" {
		fmt.Fprintln(stderr, "Usage: gistclaw run [--config path] <objective>")
		return 1
	}

	application, err := loadPreparedApp(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
		return 1
	}
	defer func() { _ = application.Stop() }()

	run, err := application.RunTask(context.Background(), objective)
	if err != nil {
		fmt.Fprintf(stderr, "run failed: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "run_id: %s\nstatus: %s\n", run.ID, run.Status)
	return 0
}

func runInspect(configPath string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, "Usage: gistclaw inspect <subcommand>\n\nSubcommands:\n  status        Show active runs, interrupted runs, approvals, and storage health\n  runs          List all runs with status\n  replay        Print replay for a run\n  config-paths  Print installer-owned directories for a config file\n  systemd-unit  Print the canonical systemd service unit\n  token         Print admin token\n")
		return 1
	}

	switch args[0] {
	case "config-paths":
		cfg, err := app.LoadInstallConfig(configPath)
		if err != nil {
			fmt.Fprintf(stderr, "inspect config-paths failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "state_dir: %s\n", cfg.StateDir)
		fmt.Fprintf(stdout, "database_dir: %s\n", filepath.Dir(cfg.DatabasePath))
		fmt.Fprintf(stdout, "workspace_root: %s\n", cfg.WorkspaceRoot)
		return 0
	case "systemd-unit":
		fmt.Fprint(stdout, app.RenderSystemdUnit(systemdBinaryPath(), systemdConfigPath()))
		return 0
	case "status":
		application, err := loadApp(configPath)
		if err != nil {
			fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
			return 1
		}
		defer func() { _ = application.Stop() }()
		status, err := application.InspectStatus(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "inspect status failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "active_runs: %d\ninterrupted_runs: %d\npending_approvals: %d\n", status.ActiveRuns, status.InterruptedRuns, status.PendingApprovals)
		fmt.Fprintf(stdout, "database_bytes: %d\nwal_bytes: %d\nfree_disk_bytes: %d\nbackup_status: %s\n", status.Storage.DatabaseBytes, status.Storage.WALBytes, status.Storage.FreeDiskBytes, status.Storage.BackupStatus)
		if status.Storage.LatestBackupAt != nil {
			fmt.Fprintf(stdout, "latest_backup_at: %s\n", status.Storage.LatestBackupAt.Format(time.RFC3339))
		}
		if status.Storage.LatestBackupPath != "" {
			fmt.Fprintf(stdout, "latest_backup_path: %s\n", status.Storage.LatestBackupPath)
		}
		if len(status.Storage.Warnings) > 0 {
			fmt.Fprintf(stdout, "storage_warnings: %s\n", joinStorageWarnings(status.Storage.Warnings))
		}
		return 0
	case "runs":
		application, err := loadApp(configPath)
		if err != nil {
			fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
			return 1
		}
		defer func() { _ = application.Stop() }()
		runs, err := application.ListRuns(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "inspect runs failed: %v\n", err)
			return 1
		}
		for _, run := range runs {
			fmt.Fprintf(stdout, "%s %s %s\n", run.ID, run.Status, run.Objective)
		}
		return 0
	case "replay":
		application, err := loadApp(configPath)
		if err != nil {
			fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
			return 1
		}
		defer func() { _ = application.Stop() }()
		if len(args) < 2 {
			fmt.Fprintln(stderr, "Usage: gistclaw inspect replay <run_id>")
			return 1
		}
		runReplay, err := application.LoadReplay(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(stderr, "inspect replay failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "run_id: %s\nstatus: %s\n", runReplay.RunID, runReplay.Status)
		for _, event := range runReplay.Events {
			fmt.Fprintf(stdout, "%s %s\n", event.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), event.Kind)
		}
		return 0
	case "token":
		application, err := loadApp(configPath)
		if err != nil {
			fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
			return 1
		}
		defer func() { _ = application.Stop() }()
		token, err := application.AdminToken(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "inspect token failed: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, token)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown inspect subcommand: %s\n", args[0])
		return 1
	}
}

func parseConfigPath(args []string) (string, []string, error) {
	configPath := os.Getenv("GISTCLAW_CONFIG")
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-c", "--config":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("missing value for %s", args[i])
			}
			configPath = args[i+1]
			i++
		default:
			rest = append(rest, args[i])
		}
	}

	if configPath == "" {
		configPath = app.DefaultConfigPath()
	}

	return configPath, rest, nil
}

func loadApp(configPath string) (*app.App, error) {
	cfg, err := app.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	application, err := app.Bootstrap(cfg)
	if err != nil {
		return nil, err
	}
	return application, nil
}

func loadPreparedApp(configPath string) (*app.App, error) {
	application, err := loadApp(configPath)
	if err != nil {
		return nil, err
	}
	if err := application.Prepare(context.Background()); err != nil {
		_ = application.Stop()
		return nil, err
	}
	return application, nil
}

func joinStorageWarnings(warnings []store.HealthWarning) string {
	if len(warnings) == 0 {
		return ""
	}

	parts := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		parts = append(parts, string(warning))
	}
	return strings.Join(parts, ",")
}

func systemdBinaryPath() string {
	if path := strings.TrimSpace(os.Getenv("GISTCLAW_SYSTEMD_BINARY_PATH")); path != "" {
		return path
	}
	return app.SystemdBinaryPath
}

func systemdConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("GISTCLAW_SYSTEMD_CONFIG_PATH")); path != "" {
		return path
	}
	return app.SystemdConfigPath
}
