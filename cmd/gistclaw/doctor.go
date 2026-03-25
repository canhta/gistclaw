package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/canhta/gistclaw/internal/app"
	"github.com/canhta/gistclaw/internal/store"
)

// runDoctor runs six health checks and prints a structured report.
// Exits 0 if all checks are PASS or WARN; exits 1 if any check is FAIL.
func runDoctor(configPath string, stdout, stderr io.Writer) int {
	type check struct {
		name   string
		status string // PASS, FAIL, WARN, SKIP
		detail string
	}

	checks := make([]check, 0, 6)
	anyFail := false

	// 1. Config file parses without error.
	cfg, cfgErr := app.LoadConfigRaw(configPath)
	if cfgErr != nil {
		checks = append(checks, check{name: "config", status: "FAIL", detail: cfgErr.Error()})
		anyFail = true
	} else {
		checks = append(checks, check{name: "config", status: "PASS", detail: configPath})
	}

	// 2. Database opens and pings.
	db, dbErr := store.Open(cfg.DatabasePath)
	if dbErr != nil {
		checks = append(checks, check{name: "database", status: "FAIL", detail: dbErr.Error()})
		anyFail = true
	} else {
		if err := db.RawDB().PingContext(context.Background()); err != nil {
			checks = append(checks, check{name: "database", status: "FAIL", detail: err.Error()})
			anyFail = true
		} else {
			checks = append(checks, check{name: "database", status: "PASS", detail: cfg.DatabasePath})
		}
		_ = db.Close()
	}

	// 3. Provider configured.
	if cfg.Provider.Name == "" && cfg.Provider.APIKey == "" {
		checks = append(checks, check{name: "provider", status: "FAIL", detail: "no provider configured in config"})
		anyFail = true
	} else {
		name := cfg.Provider.Name
		if name == "" {
			name = "anthropic"
		}
		checks = append(checks, check{name: "provider", status: "PASS", detail: name})
	}

	// 4. Workspace root exists and is writable.
	if cfg.WorkspaceRoot == "" {
		checks = append(checks, check{name: "workspace", status: "FAIL", detail: "workspace_root not configured"})
		anyFail = true
	} else if _, err := os.Stat(cfg.WorkspaceRoot); err != nil {
		checks = append(checks, check{name: "workspace", status: "FAIL", detail: fmt.Sprintf("path not found: %s", cfg.WorkspaceRoot)})
		anyFail = true
	} else {
		tmp, err := os.CreateTemp(cfg.WorkspaceRoot, ".gistclaw-doctor-*")
		if err != nil {
			checks = append(checks, check{name: "workspace", status: "FAIL", detail: fmt.Sprintf("not writable: %v", err)})
			anyFail = true
		} else {
			tmp.Close()
			os.Remove(tmp.Name())
			checks = append(checks, check{name: "workspace", status: "PASS", detail: cfg.WorkspaceRoot})
		}
	}

	// 5. Telegram (optional) — prefer YAML config and fall back to DB-backed settings.
	tgToken := cfg.Telegram.BotToken
	if tgToken == "" {
		tgToken = lookupSettingFromDB(cfg.DatabasePath, "telegram_bot_token")
	}
	if tgToken == "" {
		// No token — skip check entirely.
	} else {
		apiURL := "https://api.telegram.org/bot" + tgToken + "/getMe"
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(apiURL)
		if err != nil {
			checks = append(checks, check{name: "telegram", status: "WARN", detail: fmt.Sprintf("getMe: %v", err)})
		} else {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				checks = append(checks, check{name: "telegram", status: "PASS", detail: "getMe ok"})
			} else {
				checks = append(checks, check{name: "telegram", status: "WARN", detail: fmt.Sprintf("getMe status %d", resp.StatusCode)})
			}
		}
	}

	// 6. Disk space advisory (500 MB threshold).
	diskDir := cfg.DatabasePath
	for i := len(diskDir) - 1; i >= 0; i-- {
		if diskDir[i] == '/' || diskDir[i] == os.PathSeparator {
			diskDir = diskDir[:i]
			break
		}
	}
	if diskDir == "" {
		diskDir = "."
	}
	var stat syscall.Statfs_t
	if sErr := syscall.Statfs(diskDir, &stat); sErr == nil {
		available := stat.Bavail * uint64(stat.Bsize)
		const low = 500 * 1024 * 1024
		if available < low {
			checks = append(checks, check{name: "disk", status: "WARN", detail: fmt.Sprintf("available %d bytes (below 500 MB)", available)})
		} else {
			checks = append(checks, check{name: "disk", status: "PASS", detail: fmt.Sprintf("available %.1f GB", float64(available)/1e9)})
		}
	} else {
		checks = append(checks, check{name: "disk", status: "WARN", detail: "could not determine disk space"})
	}

	// Print report.
	for _, c := range checks {
		fmt.Fprintf(stdout, "%-12s %s  %s\n", c.name, c.status, c.detail)
	}

	if anyFail {
		return 1
	}
	return 0
}

// lookupSettingFromDB reads a setting from the database without going through
// the full bootstrap (used by doctor to check optional settings).
func lookupSettingFromDB(dbPath, key string) string {
	db, err := store.Open(dbPath)
	if err != nil {
		return ""
	}
	defer db.Close()
	var value string
	_ = db.RawDB().QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	return value
}
