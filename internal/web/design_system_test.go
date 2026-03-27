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
		".shell-project-switcher {",
		".shell-project-select.shell-toolbar-control {",
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
		`onchange="this.form.requestSubmit()"`,
		"{{range .Navigation.Groups}}",
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

func TestLayoutWrapsMonoContentToAvoidResponsiveOverflow(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(templatePath(t, "layout.html"))
	if err != nil {
		t.Fatalf("read layout template: %v", err)
	}

	content := string(body)
	for _, want := range []string{
		".mono {",
		"overflow-wrap: anywhere;",
		"word-break: break-word;",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected layout template to contain %q", want)
		}
	}
}

func TestLayoutShowsDirectoryCardsAtTabletWidths(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(templatePath(t, "layout.html"))
	if err != nil {
		t.Fatalf("read layout template: %v", err)
	}

	content := string(body)
	for _, want := range []string{
		"@media (max-width: 959px) {",
		".desktop-table {",
		".directory-card-list {",
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
			`data-confirm="Allow this change?"`,
			`data-confirm="Deny this change?"`,
		},
		"routes_deliveries.html": {
			`data-confirm="Send this note?"`,
			`data-confirm="Disconnect this route?"`,
			`data-confirm="Retry this delivery?"`,
		},
		"memory.html": {
			`data-confirm="Forget this memory?"`,
			`data-confirm="Save this edit?"`,
		},
		"onboarding.html": {
			`data-confirm="Use this repo?"`,
			`data-confirm="Start this preview?"`,
		},
		"run_submit.html": {
			`data-confirm="Start this task?"`,
		},
		"session_detail.html": {
			`data-confirm="Send this follow-up?"`,
			`data-confirm="Retry this delivery?"`,
		},
		"settings.html": {
			`data-confirm="Save these settings?"`,
			`data-confirm="Update the Telegram token?"`,
		},
		"team.html": {
			`data-confirm="Use this setup?"`,
			`data-confirm="Create this setup?"`,
			`data-confirm="Copy this setup?"`,
			`data-confirm="Delete this setup?"`,
			`data-confirm="Add another agent?"`,
			`data-confirm="Import this setup file? Unsaved edits will be replaced."`,
			`data-confirm="Remove {{.ID}} from this setup? Save to apply the change."`,
			`data-confirm="Save this setup?"`,
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

func TestTemplatesUseTaskFramedCopy(t *testing.T) {
	t.Parallel()

	type copyExpectations struct {
		wants    []string
		unwanted []string
	}

	cases := map[string]copyExpectations{
		"login.html": {
			wants: []string{
				"Connect this browser to your local GistClaw workspace.",
				"Use the admin password to open runs, approvals, and settings.",
			},
			unwanted: []string{
				"Unlock the local operator runtime for this browser.",
			},
		},
		"onboarding.html": {
			wants: []string{
				"Pick the project GistClaw should work in.",
				"Start with a preview task. You can inspect the result before files change.",
				"Preview only. Files stay untouched.",
			},
			unwanted: []string{
				"bind an existing repo",
			},
		},
		"runs.html": {
			wants: []string{
				"Recent work, blockers, and finished tasks for this project.",
				"See what is running, waiting on you, or done.",
			},
			unwanted: []string{
				"The operational queue for recent runs.",
			},
		},
		"run_detail.html": {
			wants: []string{
				"Source",
				"Attention",
				"Assigned Team",
			},
			unwanted: []string{
				"Current state",
				"Run Contract",
			},
		},
		"run_submit.html": {
			wants: []string{
				"Describe the task you want to start.",
				"Start Task",
			},
			unwanted: []string{
				"Open a new run from the operator surface.",
			},
		},
		"sessions.html": {
			wants: []string{
				"Active agent conversations for this project.",
				"Lead agent",
				"Specialist agent",
			},
			unwanted: []string{
				"assistant front session",
			},
		},
		"session_detail.html": {
			wants: []string{
				"Send follow-up",
				"This conversation is only inside GistClaw right now.",
				"Message Failures",
			},
			unwanted: []string{
				"Wake Session",
				"Delivery Failures",
			},
		},
		"team.html": {
			wants: []string{
				"Choose the agents, roles, and handoffs used for new work.",
				"Lead agent",
				"Save Setup",
			},
			unwanted: []string{
				"agent team profile",
				"Editable runtime copy",
				"Workspace Runtime Copy",
			},
		},
		"memory.html": {
			wants: []string{
				"Saved memory that guides future work.",
				"Search Memory",
				"No memory saved yet.",
			},
			unwanted: []string{
				"runtime recovery screens",
				"No memory facts found.",
			},
		},
		"settings.html": {
			wants: []string{
				"Browser access, limits, and machine credentials.",
				"Other Signed-In Browsers",
				"Telegram Token",
			},
			unwanted: []string{
				"operator settings",
			},
		},
		"approvals.html": {
			wants: []string{
				"Actions waiting for your approval.",
				"Nothing is waiting on your approval.",
			},
			unwanted: []string{
				"approval tickets",
			},
		},
		"routes_deliveries.html": {
			wants: []string{
				"Route state, message delivery, and recovery tools.",
				"Active Routes",
				"Outgoing Messages",
			},
			unwanted: []string{
				"bound session",
				"intervention surface",
				"Route Directory",
				"Delivery Queue",
			},
		},
	}

	for name, tc := range cases {
		name := name
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(templatePath(t, name))
			if err != nil {
				t.Fatalf("read template: %v", err)
			}
			content := string(body)

			for _, want := range tc.wants {
				if !strings.Contains(content, want) {
					t.Fatalf("expected %s to contain %q", name, want)
				}
			}

			for _, unwanted := range tc.unwanted {
				if strings.Contains(content, unwanted) {
					t.Fatalf("expected %s to avoid %q", name, unwanted)
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
