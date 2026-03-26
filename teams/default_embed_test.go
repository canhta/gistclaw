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

	t.Run("embeds coordinator routing rules for specialist delegation", func(t *testing.T) {
		defaults := Default()

		body, err := fs.ReadFile(defaults, "coordinator.soul.yaml")
		if err != nil {
			t.Fatalf("read embedded coordinator.soul.yaml: %v", err)
		}

		text := string(body)
		for _, want := range []string{
			"must route external research through researcher",
			"must route workspace writes through patcher",
			"must not claim a specialist acted unless a child run exists",
			"reviewer and verifier may run in parallel only after patcher work lands",
			"workflow:",
			"output_contract:",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("expected embedded coordinator prompt to contain %q, got:\n%s", want, text)
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
			"if coder_exec is unavailable or blocked, surface that explicitly to the coordinator",
			"treat coder_exec output as the primary success evidence",
			"prefer targeted list_dir, grep_search, or syntax checks over rereading generated files end to end",
			"leave deeper review and verification to reviewer and verifier",
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
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("expected embedded verifier prompt to contain %q, got:\n%s", want, text)
			}
		}
	})
}
