package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/canhta/gistclaw/internal/app"
	"github.com/canhta/gistclaw/internal/scheduler"
	securitypkg "github.com/canhta/gistclaw/internal/security"
	"github.com/canhta/gistclaw/internal/store"
	toolspkg "github.com/canhta/gistclaw/internal/tools"
)

// runDoctor runs operator health checks and prints a structured report.
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
	auditReport := securitypkg.Report{}
	if cfgErr != nil {
		checks = append(checks, check{name: "config", status: "FAIL", detail: cfgErr.Error()})
		anyFail = true
		for _, c := range checks {
			fmt.Fprintf(stdout, "%-12s %s  %s\n", c.name, c.status, c.detail)
		}
		return 1
	}
	checks = append(checks, check{name: "config", status: "PASS", detail: configPath})
	auditReport = securitypkg.RunAudit(securitypkg.Input{
		Config:            cfg,
		AdminTokenPresent: true,
	})

	// 2. Database opens and pings.
	var db *store.DB
	db, dbErr := store.Open(cfg.DatabasePath)
	if dbErr != nil {
		checks = append(checks, check{name: "database", status: "FAIL", detail: dbErr.Error()})
		anyFail = true
	} else {
		defer db.Close()
		if err := db.RawDB().PingContext(context.Background()); err != nil {
			checks = append(checks, check{name: "database", status: "FAIL", detail: err.Error()})
			anyFail = true
		} else {
			checks = append(checks, check{name: "database", status: "PASS", detail: cfg.DatabasePath})
		}
	}

	// 3. Provider configured.
	if findingDetails := joinFindingDetails(findingsBySubject(auditReport, "provider")); findingDetails != "" {
		checks = append(checks, check{name: "provider", status: "FAIL", detail: findingDetails})
		anyFail = true
	} else {
		name := cfg.Provider.Name
		if name == "" {
			name = "anthropic"
		}
		checks = append(checks, check{name: "provider", status: "PASS", detail: name})
	}

	// 4. Workspace root exists and is writable.
	if findingDetails := joinFindingDetails(findingsBySubject(auditReport, "workspace")); findingDetails != "" {
		checks = append(checks, check{name: "workspace", status: "FAIL", detail: findingDetails})
		anyFail = true
	} else {
		checks = append(checks, check{name: "workspace", status: "PASS", detail: cfg.WorkspaceRoot})
	}

	// 5. Research and MCP config safety.
	if cfg.Research.Provider != "" {
		if findingDetails := joinFindingDetails(findingsBySubject(auditReport, "research")); findingDetails != "" {
			checks = append(checks, check{name: "research", status: "FAIL", detail: findingDetails})
			anyFail = true
		} else {
			checks = append(checks, check{name: "research", status: "PASS", detail: cfg.Research.Provider})
		}
	}

	for _, server := range cfg.MCP.Servers {
		if enabledMCPTools(server.Tools) == 0 {
			continue
		}
		name := "mcp:" + server.ID
		findings := findingsBySubject(auditReport, name)
		if len(findings) > 0 {
			status := "WARN"
			if findingsHaveSeverity(findings, securitypkg.SeverityFail) {
				status = "FAIL"
				anyFail = true
			}
			checks = append(checks, check{name: name, status: status, detail: joinFindingDetails(findings)})
			continue
		}
		if resolved, err := resolveBinary(server.Command[0]); err != nil {
			checks = append(checks, check{name: name, status: "WARN", detail: err.Error()})
		} else {
			checks = append(checks, check{name: name, status: "PASS", detail: resolved})
		}
	}

	// 6. Telegram (optional) — prefer YAML config and fall back to DB-backed settings.
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

	// 7. Disk space advisory (500 MB threshold).
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

	if db != nil {
		health, err := scheduler.NewStore(db).Health(context.Background(), time.Now().UTC(), 30*time.Second)
		switch {
		case err == nil:
			if health.InvalidSchedules == 0 && health.StuckDispatching == 0 && health.MissingNextRun == 0 {
				checks = append(checks, check{name: "scheduler", status: "PASS", detail: "healthy"})
			} else {
				checks = append(checks, check{
					name:   "scheduler",
					status: "WARN",
					detail: fmt.Sprintf("invalid=%d stuck_dispatching=%d missing_next_run=%d", health.InvalidSchedules, health.StuckDispatching, health.MissingNextRun),
				})
			}
		case strings.Contains(err.Error(), "no such table: schedules"), strings.Contains(err.Error(), "no such table: schedule_occurrences"):
			checks = append(checks, check{name: "scheduler", status: "SKIP", detail: "scheduler tables not initialized"})
		default:
			checks = append(checks, check{name: "scheduler", status: "FAIL", detail: err.Error()})
			anyFail = true
		}
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

func findingsBySubject(report securitypkg.Report, subject string) []securitypkg.Finding {
	findings := make([]securitypkg.Finding, 0, len(report.Findings))
	for _, finding := range report.Findings {
		if finding.Subject == subject {
			findings = append(findings, finding)
		}
	}
	return findings
}

func findingsHaveSeverity(findings []securitypkg.Finding, severity securitypkg.Severity) bool {
	for _, finding := range findings {
		if finding.Severity == severity {
			return true
		}
	}
	return false
}

func joinFindingDetails(findings []securitypkg.Finding) string {
	if len(findings) == 0 {
		return ""
	}

	parts := make([]string, 0, len(findings))
	for _, finding := range findings {
		parts = append(parts, finding.Detail)
	}
	return strings.Join(parts, "; ")
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

func enabledMCPTools(tools []toolspkg.MCPToolConfig) int {
	count := 0
	for _, tool := range tools {
		if tool.Enabled {
			count++
		}
	}
	return count
}

func resolveBinary(command string) (string, error) {
	if command == "" {
		return "", fmt.Errorf("binary is required")
	}
	if filepath.IsAbs(command) || strings.ContainsRune(command, os.PathSeparator) {
		if _, err := os.Stat(command); err != nil {
			return "", fmt.Errorf("binary not found: %s", command)
		}
		return command, nil
	}
	resolved, err := exec.LookPath(command)
	if err != nil {
		return "", fmt.Errorf("binary not found: %s", command)
	}
	return resolved, nil
}
