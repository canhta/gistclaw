package projectscope

import (
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

func TestRunCondition(t *testing.T) {
	project := model.Project{
		ID:            "proj-1",
		Name:          "alpha",
		WorkspaceRoot: "/tmp/alpha",
		CreatedAt:     time.Now().UTC(),
		LastUsedAt:    time.Now().UTC(),
	}

	t.Run("project and workspace fallback", func(t *testing.T) {
		got, args := RunCondition(project, "runs")
		want := "(runs.project_id = ? OR ((runs.project_id IS NULL OR runs.project_id = '') AND COALESCE(runs.workspace_root, '') = ?))"
		if got != want {
			t.Fatalf("expected condition %q, got %q", want, got)
		}
		if len(args) != 2 || args[0] != "proj-1" || args[1] != "/tmp/alpha" {
			t.Fatalf("unexpected args: %#v", args)
		}
	})

	t.Run("workspace only", func(t *testing.T) {
		got, args := RunCondition(model.Project{WorkspaceRoot: "/tmp/alpha"}, "scope_runs")
		want := "COALESCE(scope_runs.workspace_root, '') = ?"
		if got != want {
			t.Fatalf("expected condition %q, got %q", want, got)
		}
		if len(args) != 1 || args[0] != "/tmp/alpha" {
			t.Fatalf("unexpected args: %#v", args)
		}
	})

	t.Run("no project scope", func(t *testing.T) {
		got, args := RunCondition(model.Project{}, "runs")
		if got != "1 = 1" {
			t.Fatalf("expected no-op scope, got %q", got)
		}
		if len(args) != 0 {
			t.Fatalf("expected no args, got %#v", args)
		}
	})
}
