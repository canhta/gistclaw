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
		"control.html",
		"memory.html",
		"onboarding.html",
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
		".field {",
		".btn {",
		".btn-primary {",
		".btn-secondary {",
		".btn-danger {",
		".badge {",
		".choice-card {",
		".pager {",
		"border-radius: 0;",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected layout template to contain %q", want)
		}
	}
}
