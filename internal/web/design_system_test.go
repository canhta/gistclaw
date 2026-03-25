package web

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
)

func templatePath(t *testing.T, name string) string {
	t.Helper()

	_, currentFile, _, ok := goruntime.Caller(0)
	if !ok {
		t.Fatal("locate current file")
	}

	return filepath.Join(filepath.Dir(currentFile), "templates", name)
}

func TestTemplatesAvoidInlineStyles(t *testing.T) {
	t.Parallel()

	files := []string{
		"approvals.html",
		"memory.html",
		"onboarding.html",
		"routes_deliveries.html",
		"session_detail.html",
		"sessions.html",
		"settings.html",
	}

	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(templatePath(t, name))
			if err != nil {
				t.Fatalf("read template: %v", err)
			}
			if strings.Contains(string(body), "style=") {
				t.Fatalf("template %s must use shared brutalist classes instead of inline styles", name)
			}
		})
	}
}

func TestLayoutDefinesBrutalistPrimitives(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(templatePath(t, "layout.html"))
	if err != nil {
		t.Fatalf("read layout template: %v", err)
	}

	content := string(body)
	for _, want := range []string{
		"--approval: #b45309;",
		"--success: #15803d;",
		"--error: #dc2626;",
		":root[data-theme=\"dark\"] {",
		"@media (prefers-color-scheme: dark) {",
		".shell-nav {",
		".shell-brand-lockup {",
		".shell-subnav {",
		".shell-nav-group {",
		".shell-start-task {",
		".page-header {",
		".page-stack {",
		".page-section {",
		".run-status {",
		".theme-switcher {",
		".segmented-control {",
		".run-link {",
		".graph-diagram {",
		".graph-board {",
		".graph-node {",
		".desktop-table {",
		".directory-card-list {",
		".directory-card {",
		".field {",
		".field-label-ghost {",
		".btn {",
		".btn-primary {",
		".btn-secondary {",
		".btn-danger {",
		".btn-compact {",
		".team-summary-head {",
		".team-file-tools {",
		".team-primary-actions {",
		".metric-strip {",
		".empty-state {",
		".queue-strip {",
		".filter-action-group {",
		".session-filter-grid {",
		".session-filter-footer {",
		".badge {",
		".team-utility-bar {",
		".approval-filter-grid {",
		".approval-card-actions {",
		".choice-card {",
		".pager {",
		"event.submitter",
		"window.confirm(message)",
		"{{range .Navigation.Groups}}",
		".Navigation.StartTask.Href",
		".Navigation.Children",
		"@media (max-width: 767px) {",
		"border-radius: 0;",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected layout template to contain %q", want)
		}
	}
}

func TestLayoutDefinesUnifiedControlHeight(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(templatePath(t, "layout.html"))
	if err != nil {
		t.Fatalf("read layout template: %v", err)
	}

	content := string(body)
	for _, want := range []string{
		"--control-height: 44px;",
		"height: var(--control-height);",
		"min-height: var(--control-height);",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected layout template to contain %q", want)
		}
	}
}

func TestLayoutDefinesCompactUtilityButtonHeight(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(templatePath(t, "layout.html"))
	if err != nil {
		t.Fatalf("read layout template: %v", err)
	}

	content := string(body)
	for _, want := range []string{
		".btn-compact {",
		"min-height: 28px;",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected layout template to contain %q", want)
		}
	}
}

func TestCriticalTemplatesDefineConfirmationMessages(t *testing.T) {
	t.Parallel()

	cases := map[string][]string{
		"approvals.html": {
			`data-confirm="Approve this approval ticket? This action resolves it immediately."`,
			`data-confirm="Deny this approval ticket? This action resolves it immediately."`,
		},
		"routes_deliveries.html": {
			`data-confirm="Send this operator message into the bound session?"`,
			`data-confirm="Deactivate this route? External messages will stop flowing into the bound session."`,
			`data-confirm="Retry this terminal delivery now?"`,
		},
		"memory.html": {
			`data-confirm="Forget this memory item permanently?"`,
			`data-confirm="Save this memory edit?"`,
		},
		"onboarding.html": {
			`data-confirm="Bind this workspace for local operations?"`,
			`data-confirm="Start this preview run now?"`,
		},
		"run_submit.html": {
			`data-confirm="Start this run now?"`,
		},
		"session_detail.html": {
			`data-confirm="Wake this session with a new operator message?"`,
			`data-confirm="Retry this session delivery now?"`,
		},
		"settings.html": {
			`data-confirm="Save these workspace settings?"`,
			`data-confirm="Update the Telegram bot token?"`,
		},
		"team.html": {
			`data-confirm="Add a new team member to the editor?"`,
			`data-confirm="Import this team file into the editor? Unsaved changes in the current editor will be replaced."`,
			`data-confirm="Remove {{.ID}} from this team? This stays in the editor until you save."`,
			`data-confirm="Save this team to the workspace-owned runtime copy?"`,
		},
	}

	for name, wants := range cases {
		name := name
		wants := wants
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(templatePath(t, name))
			if err != nil {
				t.Fatalf("read template: %v", err)
			}
			content := string(body)
			for _, want := range wants {
				if !strings.Contains(content, want) {
					t.Fatalf("expected %s to contain %q", name, want)
				}
			}
		})
	}
}

func TestSessionDetailUsesSharedConfirmationHook(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(templatePath(t, "session_detail.html"))
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	if !strings.Contains(string(body), "window.gistclawConfirmSubmission(form, event.submitter)") {
		t.Fatal("expected session detail realtime submit flow to use the shared confirmation hook")
	}
}
