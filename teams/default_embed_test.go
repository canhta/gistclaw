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
}
