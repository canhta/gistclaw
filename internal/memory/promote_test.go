package memory

import (
	"testing"

	"github.com/canhta/gistclaw/internal/model"
)

func TestCandidateFromRunObjectiveExtractsDurableFactFromObjective(t *testing.T) {
	run := model.Run{
		ID:             "run-1",
		ProjectID:      "proj-alpha",
		ConversationID: "conv-1",
		AgentID:        "assistant",
	}

	candidate, ok := CandidateFromRunObjective(
		run,
		"Remember this for future runs: the repo uses pnpm workspaces.",
	)
	if !ok {
		t.Fatal("expected explicit remember request to produce a durable memory candidate")
	}
	if got := candidate.Content; got != "the repo uses pnpm workspaces." {
		t.Fatalf("expected extracted durable fact, got %q", got)
	}
	if got := candidate.DedupeKey; got == "" {
		t.Fatal("expected dedupe key to be populated")
	}
}

func TestCandidateFromRunObjectiveSkipsVagueRememberPromptWithoutDurableFact(t *testing.T) {
	run := model.Run{
		ID:             "run-1",
		ProjectID:      "proj-alpha",
		ConversationID: "conv-1",
		AgentID:        "assistant",
	}

	_, ok := CandidateFromRunObjective(
		run,
		"Remember this for future runs.",
	)
	if ok {
		t.Fatal("expected vague remember prompt without an embedded fact to be skipped")
	}
}

func TestCandidateFromRunObjectiveExtractsNaturalPromptPreferences(t *testing.T) {
	run := model.Run{
		ID:             "run-1",
		ProjectID:      "proj-alpha",
		ConversationID: "conv-1",
		AgentID:        "assistant",
	}

	candidate, ok := CandidateFromRunObjective(
		run,
		"Please create a tiny static developer notes page. Keep the tone technical and aimed at developers evaluating self-hosted assistants. If tooling is needed, prefer bun-based workflows and keep lockfile churn isolated. Use Codex CLI for code changes, then review and verify before wrapping up.",
	)
	if !ok {
		t.Fatal("expected durable preferences to be extracted from a natural prompt")
	}
	want := "Keep the tone technical and aimed at developers evaluating self-hosted assistants. If tooling is needed, prefer bun-based workflows and keep lockfile churn isolated. Use Codex CLI for code changes."
	if got := candidate.Content; got != want {
		t.Fatalf("expected prompt preference summary %q, got %q", want, got)
	}
	if got := candidate.Provenance; got != "prompt_preference_summary" {
		t.Fatalf("expected prompt preference provenance, got %q", got)
	}
}

func TestCandidateFromRunObjectiveSkipsSingleWeakStylePreference(t *testing.T) {
	run := model.Run{
		ID:             "run-1",
		ProjectID:      "proj-alpha",
		ConversationID: "conv-1",
		AgentID:        "assistant",
	}

	_, ok := CandidateFromRunObjective(
		run,
		"Create a tiny static developer notes page. Keep the tone technical.",
	)
	if ok {
		t.Fatal("expected a lone weak style hint to stay out of durable memory")
	}
}
