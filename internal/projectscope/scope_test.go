package projectscope

import (
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestRunCondition(t *testing.T) {
	project := model.Project{
		ID:   "proj-1",
		Name: "alpha",
	}

	t.Run("project id only", func(t *testing.T) {
		got, args := RunCondition(project, "runs")
		want := "runs.project_id = ?"
		if got != want {
			t.Fatalf("expected condition %q, got %q", want, got)
		}
		if len(args) != 1 || args[0] != "proj-1" {
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
