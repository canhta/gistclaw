package shippedteams

import (
	"io/fs"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	t.Run("embeds shipped default team files", func(t *testing.T) {
		defaults := Default()

		entries, err := fs.ReadDir(defaults, ".")
		if err != nil {
			t.Fatalf("read embedded team dir: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("expected embedded default team files")
		}

		body, err := fs.ReadFile(defaults, "team.yaml")
		if err != nil {
			t.Fatalf("read embedded team.yaml: %v", err)
		}
		if !strings.Contains(string(body), "name:") {
			t.Fatalf("expected embedded team.yaml to include team metadata, got:\n%s", string(body))
		}
	})

	t.Run("embeds front assistant direct-execution policy", func(t *testing.T) {
		defaults := Default()

		body, err := fs.ReadFile(defaults, "assistant.soul.yaml")
		if err != nil {
			t.Fatalf("read embedded assistant.soul.yaml: %v", err)
		}

		text := string(body)
		for _, want := range []string{
			"You are the user-facing assistant for repo tasks.",
			"Execute directly when the task is bounded and the required capabilities are already available.",
			"Delegate only when specialization, uncertainty reduction, scale, or parallelism adds clear value.",
			"prefer direct capability execution for simple, bounded tasks",
			"use short deterministic tool chains before delegating",
			"if the runtime recommends delegate or parallelize, treat that as the default unless stronger direct evidence says otherwise",
			"do not claim specialist work happened unless a delegated run exists",
			"do not mutate files directly when a write specialist is available for that job",
			"output_contract:",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("expected embedded assistant prompt to contain %q, got:\n%s", want, text)
			}
		}
	})

	t.Run("embeds patcher coder_exec guidance", func(t *testing.T) {
		defaults := Default()

		body, err := fs.ReadFile(defaults, "patcher.soul.yaml")
		if err != nil {
			t.Fatalf("read embedded patcher.soul.yaml: %v", err)
		}

		text := string(body)
		for _, want := range []string{
			"prefer coder_exec with backend codex for substantial code generation",
			"must not reconstruct codex exec flags manually when coder_exec can express the job",
			"if coder_exec is unavailable or blocked, surface that explicitly to the front assistant",
			"must not ask the operator directly",
			"treat coder_exec output as the primary success evidence",
			"prefer targeted list_dir, grep_search, or syntax checks over rereading generated files end to end",
			"leave deeper review and verification to reviewer and verifier",
			"must either execute the requested write path or report the blocker",
			"must not stop at a proposal when the delegated task is to make the change",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("expected embedded patcher prompt to contain %q, got:\n%s", want, text)
			}
		}
	})

	t.Run("embeds reviewer targeted inspection guidance", func(t *testing.T) {
		defaults := Default()

		body, err := fs.ReadFile(defaults, "reviewer.soul.yaml")
		if err != nil {
			t.Fatalf("read embedded reviewer.soul.yaml: %v", err)
		}

		text := string(body)
		for _, want := range []string{
			"prefer targeted grep or narrow read_file slices over full-file reads",
			"start with the smallest relevant files or sections first",
			"inspect CSS or JS only when a suspected issue requires it",
			"must not ask the operator directly",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("expected embedded reviewer prompt to contain %q, got:\n%s", want, text)
			}
		}
	})

	t.Run("embeds verifier targeted inspection guidance", func(t *testing.T) {
		defaults := Default()

		body, err := fs.ReadFile(defaults, "verifier.soul.yaml")
		if err != nil {
			t.Fatalf("read embedded verifier.soul.yaml: %v", err)
		}

		text := string(body)
		for _, want := range []string{
			"prefer targeted grep or narrow read_file slices over full-file reads",
			"start with the smallest relevant files or sections first",
			"inspect CSS or JS only when a requested verification step requires it",
			"must not ask the operator directly",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("expected embedded verifier prompt to contain %q, got:\n%s", want, text)
			}
		}
	})

	t.Run("embeds researcher front-assistant escalation guidance", func(t *testing.T) {
		defaults := Default()

		body, err := fs.ReadFile(defaults, "researcher.soul.yaml")
		if err != nil {
			t.Fatalf("read embedded researcher.soul.yaml: %v", err)
		}

		text := string(body)
		for _, want := range []string{
			"do not ask the operator directly",
			"route unresolved blockers back through the front assistant",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("expected embedded researcher prompt to contain %q, got:\n%s", want, text)
			}
		}
	})
}
