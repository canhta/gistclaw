package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/connectors/zalopersonal"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/scheduler"
	"github.com/canhta/gistclaw/internal/store"
)

func TestDoctor_AllChecksPass(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)

	var stdout, stderr bytes.Buffer
	runDoctor(testOptions(cfgPath), &stdout, &stderr)
	output := stdout.String()
	if !strings.Contains(output, "PASS") {
		t.Errorf("expected PASS in output:\n%s", output)
	}
}

func TestDoctor_PrintsConnectorHealthSummary(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := strings.Join([]string{
		"database_path: " + dbPath,
		"storage_root: " + workspaceRoot,
		"provider:",
		"  name: openai",
		"  api_key: sk-test",
		"whatsapp:",
		"  phone_number_id: phone-123",
		"  access_token: wa-token",
		"  verify_token: verify-token",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runDoctor(testOptions(cfgPath), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"connector:whatsapp", "awaiting webhook traffic"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in doctor output:\n%s", want, output)
		}
	}
	if !strings.Contains(output, "connector:whatsapp SKIP") {
		t.Fatalf("expected unknown connector health to render as SKIP, got:\n%s", output)
	}
}

func TestDoctor_PrintsZaloPersonalConnectorHealthAndSecurityWarning(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := strings.Join([]string{
		"database_path: " + dbPath,
		"storage_root: " + workspaceRoot,
		"provider:",
		"  name: openai",
		"  api_key: sk-test",
		"zalo_personal:",
		"  enabled: true",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	raw, err := json.Marshal(model.ConnectorHealthSnapshot{
		ConnectorID: "zalo_personal",
		State:       model.ConnectorHealthHealthy,
		Summary:     "connected",
		CheckedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal connector health snapshot: %v", err)
	}
	if _, err := db.RawDB().Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		"connector_health.zalo_personal",
		string(raw),
	); err != nil {
		t.Fatalf("seed connector health: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runDoctor(testOptions(cfgPath), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"connector:zalo_personal",
		"connected",
		"security:zalo_personal",
		"reverse-engineered personal-account behavior",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in doctor output:\n%s", want, output)
		}
	}
	if strings.Contains(output, "awaiting first authentication") {
		t.Fatalf("expected persisted connector health to override cold-start summary, got:\n%s", output)
	}
}

func TestDoctor_PrintsStoredZaloCredentialsBeforeFirstDaemonStart(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := strings.Join([]string{
		"database_path: " + dbPath,
		"storage_root: " + workspaceRoot,
		"provider:",
		"  name: openai",
		"  api_key: sk-test",
		"zalo_personal:",
		"  enabled: true",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	if err := zalopersonal.SaveStoredCredentials(context.Background(), db, zalopersonal.StoredCredentials{
		AccountID:   "zalo-account",
		DisplayName: "Zalo User",
		IMEI:        "imei-123",
		Cookie:      "cookie=abc",
		UserAgent:   "gistclaw-test",
		Language:    "vi",
	}); err != nil {
		t.Fatalf("SaveStoredCredentials: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runDoctor(testOptions(cfgPath), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"connector:zalo_personal",
		"credentials stored",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in doctor output:\n%s", want, output)
		}
	}
	if strings.Contains(output, "awaiting first authentication") {
		t.Fatalf("expected stored credentials to override cold-start summary, got:\n%s", output)
	}
}

func TestDoctor_MissingStorageRootFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, "/nonexistent/workspace/path-xyz-99")

	var stdout, stderr bytes.Buffer
	code := runDoctor(testOptions(cfgPath), &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit for missing storage_root")
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in output:\n%s", stdout.String())
	}
}

func TestDoctor_MissingProviderFails(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := "database_path: " + dbPath + "\nstorage_root: " + workspaceRoot + "\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runDoctor(testOptions(cfgPath), &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit for missing provider")
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in output:\n%s", stdout.String())
	}
}

func TestDoctor_LowDiskIsWarn(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)

	var stdout, stderr bytes.Buffer
	runDoctor(testOptions(cfgPath), &stdout, &stderr)
	output := stdout.String()
	if !strings.Contains(output, "storage") {
		t.Errorf("expected storage check line in output:\n%s", output)
	}
}

func TestDoctor_TelegramMissingIsSkipped(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)

	var stdout, stderr bytes.Buffer
	runDoctor(testOptions(cfgPath), &stdout, &stderr)
	output := stdout.String()
	// When no telegram token is set, the telegram check should not appear.
	if strings.Contains(output, "telegram") {
		t.Errorf("telegram check should be skipped when no token set, got:\n%s", output)
	}
}

func TestDoctor_TelegramConfiguredInYAMLIsChecked(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := strings.Join([]string{
		"database_path: " + dbPath,
		"storage_root: " + workspaceRoot,
		"provider:",
		"  name: openai",
		"  api_key: sk-test",
		"telegram:",
		"  bot_token: telegram-test-token",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	runDoctor(testOptions(cfgPath), &stdout, &stderr)
	output := stdout.String()
	if !strings.Contains(output, "telegram") {
		t.Errorf("expected telegram check line when token is configured in YAML, got:\n%s", output)
	}
}

func TestDoctor_BadConfigFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDoctor(testOptions("/nonexistent/config.yaml"), &stdout, &stderr)
	if code == 0 {
		t.Error("expected non-zero exit for missing config file")
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in output:\n%s", stdout.String())
	}
}

func TestDoctor_BadConfigStopsAfterConfigFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDoctor(testOptions("/nonexistent/config.yaml"), &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for missing config file")
	}

	output := stdout.String()
	if strings.Contains(output, "provider") {
		t.Fatalf("expected provider check to be skipped when config load fails, got:\n%s", output)
	}
	if strings.Contains(output, "storage_root") {
		t.Fatalf("expected storage_root check to be skipped when config load fails, got:\n%s", output)
	}
	if strings.Contains(output, "database") {
		t.Fatalf("expected database check to be skipped when config load fails, got:\n%s", output)
	}
}

func TestDoctor_ResearchProviderWithoutAPIKeyFails(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := strings.Join([]string{
		"database_path: " + dbPath,
		"storage_root: " + workspaceRoot,
		"provider:",
		"  name: anthropic",
		"  api_key: sk-test",
		"research:",
		"  provider: tavily",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runDoctor(testOptions(cfgPath), &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for invalid research config")
	}
	if !strings.Contains(stdout.String(), "research") || !strings.Contains(stdout.String(), "FAIL") {
		t.Fatalf("expected research FAIL check, got:\n%s", stdout.String())
	}
}

func TestDoctor_MissingMCPBinaryWarns(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")

	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := strings.Join([]string{
		"database_path: " + dbPath,
		"storage_root: " + workspaceRoot,
		"provider:",
		"  name: anthropic",
		"  api_key: sk-test",
		"mcp:",
		"  servers:",
		"    - id: github",
		"      transport: stdio",
		"      command: [\"definitely-not-a-real-mcp-binary\"]",
		"      tools:",
		"        - name: search_repositories",
		"          alias: github_search_repositories",
		"          risk: low",
		"          enabled: true",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runDoctor(testOptions(cfgPath), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected warn-only exit code, got %d", code)
	}
	if !strings.Contains(stdout.String(), "mcp:github") || !strings.Contains(stdout.String(), "WARN") {
		t.Fatalf("expected mcp warn line, got:\n%s", stdout.String())
	}
}

func TestDoctor_ResearchAndMCPChecksAreSkippedWhenUnconfigured(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)

	var stdout, stderr bytes.Buffer
	runDoctor(testOptions(cfgPath), &stdout, &stderr)
	output := stdout.String()
	if strings.Contains(output, "research") || strings.Contains(output, "mcp:") {
		t.Fatalf("expected research and mcp checks to be skipped, got:\n%s", output)
	}
}

func TestDoctor_WarnsOnBrokenSchedulerState(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open db: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate db: %v", err)
	}

	s := scheduler.NewStore(db)
	ctx := context.Background()
	invalidSchedule, err := s.CreateSchedule(ctx, scheduler.CreateScheduleInput{
		ID:        "sched-invalid",
		Name:      "Invalid cron",
		Objective: "Inspect invalid cron state",
		CWD:       workspaceRoot,
		Spec: scheduler.ScheduleSpec{
			Kind:     scheduler.ScheduleKindCron,
			CronExpr: "0 * * * *",
			Timezone: "UTC",
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateSchedule invalid seed: %v", err)
	}
	stuckSchedule, err := s.CreateSchedule(ctx, scheduler.CreateScheduleInput{
		ID:        "sched-stuck",
		Name:      "Stuck dispatch",
		Objective: "Inspect stuck dispatch state",
		CWD:       workspaceRoot,
		Spec: scheduler.ScheduleSpec{
			Kind: scheduler.ScheduleKindAt,
			At:   "2030-01-02T00:00:00Z",
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateSchedule stuck seed: %v", err)
	}
	missingSchedule, err := s.CreateSchedule(ctx, scheduler.CreateScheduleInput{
		ID:        "sched-missing-next",
		Name:      "Missing next run",
		Objective: "Inspect missing next run state",
		CWD:       workspaceRoot,
		Spec: scheduler.ScheduleSpec{
			Kind: scheduler.ScheduleKindAt,
			At:   "2030-01-03T00:00:00Z",
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateSchedule missing-next seed: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.RawDB().ExecContext(ctx,
		`UPDATE schedules
		    SET schedule_cron_expr = ?, next_run_at = ?, updated_at = ?
		  WHERE id = ?`,
		"not a cron expression",
		now.Add(time.Hour),
		now,
		invalidSchedule.ID,
	); err != nil {
		t.Fatalf("break cron expression: %v", err)
	}
	if _, err := db.RawDB().ExecContext(ctx,
		`INSERT INTO schedule_occurrences
		 (id, schedule_id, slot_at, thread_id, status, skip_reason, run_id, conversation_id, error, started_at, finished_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', '', '', '', NULL, NULL, ?, ?)`,
		"occ-stuck",
		stuckSchedule.ID,
		now.Add(-2*time.Minute),
		now.Add(-2*time.Minute).Format(time.RFC3339Nano),
		scheduler.OccurrenceDispatching,
		now.Add(-2*time.Minute),
		now.Add(-2*time.Minute),
	); err != nil {
		t.Fatalf("insert stuck dispatching occurrence: %v", err)
	}
	if _, err := db.RawDB().ExecContext(ctx,
		`UPDATE schedules
		    SET next_run_at = NULL, updated_at = ?
		  WHERE id = ?`,
		now,
		missingSchedule.ID,
	); err != nil {
		t.Fatalf("clear next_run_at: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runDoctor(testOptions(cfgPath), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected warn-only exit code, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{
		"scheduler",
		"WARN",
		"invalid=1",
		"stuck_dispatching=1",
		"missing_next_run=1",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestDoctor_PrintsStorageHealthSummary(t *testing.T) {
	workspaceRoot := t.TempDir()
	exec.Command("git", "init", workspaceRoot).Run()
	dbPath := filepath.Join(t.TempDir(), "gistclaw.db")
	cfgPath := makeValidConfig(t, dbPath, workspaceRoot)
	seedDB(t, dbPath)

	backupPath := filepath.Join(filepath.Dir(dbPath), "gistclaw.20260326-010203.db.bak")
	if err := os.WriteFile(backupPath, []byte("backup"), 0o644); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	backupAt := time.Date(2026, time.March, 26, 1, 2, 3, 0, time.UTC)
	if err := os.Chtimes(backupPath, backupAt, backupAt); err != nil {
		t.Fatalf("chtimes backup: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runDoctor(testOptions(cfgPath), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected warn-or-pass exit code, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	for _, want := range []string{"storage", "db=", "backup=fresh"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, stdout.String())
		}
	}
}
