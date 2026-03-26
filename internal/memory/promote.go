package memory

import (
	"context"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
)

// authorizedScopes lists the scope levels ordered from narrowest to broadest.
var authorizedScopes = map[string]int{
	"local": 0,
	"team":  1,
}

// AuthorizeEscalation returns an error if promoting from currentScope to
// targetScope is not permitted. The run engine calls this before invoking
// WriteFact with a higher scope; the memory store does not call it.
func AuthorizeEscalation(currentScope, targetScope string) error {
	curLevel, curOK := authorizedScopes[currentScope]
	tgtLevel, tgtOK := authorizedScopes[targetScope]
	if !curOK {
		return fmt.Errorf("memory: unknown current scope %q", currentScope)
	}
	if !tgtOK {
		return fmt.Errorf("memory: unknown target scope %q", targetScope)
	}
	if tgtLevel <= curLevel {
		return fmt.Errorf("memory: scope escalation from %q to %q is not an escalation", currentScope, targetScope)
	}
	return nil
}

var explicitMemoryIntentPhrases = []string{
	"remember this",
	"remember that",
	"remember for future",
	"remember for future runs",
	"save this to memory",
	"store this in memory",
	"keep this in memory",
	"memorize this",
}

var explicitMemoryExtractionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)^\s*remember(?:\s+this)?(?:\s+for\s+future(?:\s+runs?)?)?\s*[:\-]\s*(.+?)\s*$`),
	regexp.MustCompile(`(?is)^\s*remember\s+that\s+(.+?)\s*$`),
	regexp.MustCompile(`(?is)^\s*(?:save|store|keep|memorize)\s+this(?:\s+(?:to|in)\s+memory)?(?:\s+for\s+future(?:\s+runs?)?)?\s*[:\-]\s*(.+?)\s*$`),
	regexp.MustCompile(`(?is)^\s*(?:save|store|keep)\s+(.+?)\s+(?:to|in)\s+memory\s*$`),
}

var naturalPromptSentenceSplit = regexp.MustCompile(`[.!?\n]+`)

func CandidateFromRunObjective(run model.Run, objective string) (model.MemoryCandidate, bool) {
	if run.ParentRunID != "" {
		return model.MemoryCandidate{}, false
	}
	projectID := strings.TrimSpace(run.ProjectID)
	if projectID == "" {
		return model.MemoryCandidate{}, false
	}
	content := ""
	provenance := ""
	confidence := 0.0
	dedupeKey := ""

	if hasExplicitMemoryIntent(objective) {
		content = extractExplicitMemoryFact(objective)
		provenance = "explicit_memory_request"
		confidence = 0.95
		dedupeKey = explicitMemoryDedupeKey(projectID, content)
	}

	if content == "" {
		content = extractPromptPreferenceMemory(objective)
		if content == "" {
			return model.MemoryCandidate{}, false
		}
		provenance = "prompt_preference_summary"
		confidence = 0.82
		dedupeKey = promptPreferenceDedupeKey(projectID, content)
	}

	return model.MemoryCandidate{
		ProjectID:      projectID,
		AgentID:        run.AgentID,
		Scope:          "team",
		Content:        content,
		Provenance:     provenance,
		Confidence:     confidence,
		DedupeKey:      dedupeKey,
		ConversationID: run.ConversationID,
	}, true
}

func hasExplicitMemoryIntent(objective string) bool {
	normalized := normalizeMemoryText(objective)
	for _, phrase := range explicitMemoryIntentPhrases {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}

func explicitMemoryDedupeKey(projectID, content string) string {
	return memoryDedupeKey("explicit_memory", projectID, content)
}

func promptPreferenceDedupeKey(projectID, content string) string {
	return memoryDedupeKey("prompt_preference", projectID, content)
}

func memoryDedupeKey(prefix, projectID, content string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(projectID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(normalizeMemoryText(content)))
	return fmt.Sprintf("%s:%x", prefix, h.Sum64())
}

func normalizeMemoryText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func extractExplicitMemoryFact(objective string) string {
	trimmed := strings.TrimSpace(objective)
	for _, pattern := range explicitMemoryExtractionPatterns {
		matches := pattern.FindStringSubmatch(trimmed)
		if len(matches) < 2 {
			continue
		}
		content := cleanMemoryFactText(matches[1])
		if content != "" {
			return content
		}
	}
	return ""
}

func cleanMemoryFactText(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.Trim(trimmed, "\"'`")
	return strings.Join(strings.Fields(trimmed), " ")
}

func extractPromptPreferenceMemory(objective string) string {
	parts := naturalPromptSentenceSplit.Split(objective, -1)
	candidates := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	strongCount := 0

	add := func(content string, strong bool) {
		content = cleanPromptPreferenceSentence(content)
		if content == "" {
			return
		}
		key := normalizeMemoryText(content)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, content)
		if strong {
			strongCount++
		}
	}

	for _, part := range parts {
		sentence := cleanMemoryFactText(part)
		if sentence == "" {
			continue
		}
		normalized := normalizeMemoryText(sentence)

		switch {
		case strings.Contains(normalized, "keep the tone") || (strings.Contains(normalized, "tone") && strings.Contains(normalized, "aimed at")):
			add(sentence, false)
		}

		switch {
		case strings.Contains(normalized, "if tooling is needed") && strings.Contains(normalized, "prefer "):
			add(sentence, true)
		case strings.Contains(normalized, "prefer ") &&
			(strings.Contains(normalized, "workflow") || strings.Contains(normalized, "bun") || strings.Contains(normalized, "pnpm") || strings.Contains(normalized, "npm")):
			add(sentence, true)
		}

		switch {
		case strings.Contains(normalized, "use codex cli for code changes"):
			add("Use Codex CLI for code changes", true)
		case strings.Contains(normalized, "use claude code for code changes"):
			add("Use Claude Code for code changes", true)
		}

		if strings.Contains(normalized, "keep lockfile churn isolated") && !strings.Contains(normalized, "prefer ") {
			add("Keep lockfile churn isolated", true)
		}
	}

	if len(candidates) == 0 {
		return ""
	}
	if strongCount == 0 && len(candidates) < 2 {
		return ""
	}
	return strings.Join(candidates, " ")
}

func cleanPromptPreferenceSentence(value string) string {
	cleaned := cleanMemoryFactText(value)
	if cleaned == "" {
		return ""
	}
	lower := strings.ToLower(cleaned)
	switch {
	case strings.HasPrefix(lower, "please "):
		cleaned = strings.TrimSpace(cleaned[len("Please "):])
	case strings.HasPrefix(lower, "also "):
		cleaned = strings.TrimSpace(cleaned[len("Also "):])
	}
	if idx := strings.Index(cleaned, ", then "); idx >= 0 {
		cleaned = strings.TrimSpace(cleaned[:idx])
	}
	if idx := strings.Index(cleaned, " then "); idx >= 0 && strings.HasPrefix(strings.ToLower(cleaned), "use ") {
		cleaned = strings.TrimSpace(cleaned[:idx])
	}
	if !strings.HasSuffix(cleaned, ".") {
		cleaned += "."
	}
	return cleaned
}

func (s *Store) PromoteCandidate(ctx context.Context, candidate model.MemoryCandidate) error {
	if candidate.ProjectID == "" {
		return fmt.Errorf("memory: promote: project_id required")
	}
	if candidate.AgentID == "" {
		return fmt.Errorf("memory: promote: agent_id required")
	}
	if candidate.Scope == "" {
		return fmt.Errorf("memory: promote: scope required")
	}
	if candidate.Content == "" {
		return fmt.Errorf("memory: promote: content required")
	}
	if candidate.DedupeKey == "" {
		return fmt.Errorf("memory: promote: dedupe_key required")
	}

	existing, found, err := s.getByDedupeKey(ctx, candidate.ProjectID, candidate.DedupeKey)
	if err != nil {
		return fmt.Errorf("memory: promote lookup: %w", err)
	}

	id := memGenerateID()
	if found {
		id = existing.ID
		if existing.Source == "human" {
			return nil
		}
		err = s.UpdateFact(ctx, model.MemoryItem{
			ID:         existing.ID,
			ProjectID:  existing.ProjectID,
			AgentID:    existing.AgentID,
			Scope:      existing.Scope,
			Content:    candidate.Content,
			Source:     "model",
			Provenance: candidate.Provenance,
			Confidence: candidate.Confidence,
		})
		if err != nil {
			return fmt.Errorf("memory: promote update: %w", err)
		}
	} else {
		err = s.WriteFact(ctx, model.MemoryItem{
			ID:         id,
			ProjectID:  candidate.ProjectID,
			AgentID:    candidate.AgentID,
			Scope:      candidate.Scope,
			Content:    candidate.Content,
			Source:     "model",
			Provenance: candidate.Provenance,
			Confidence: candidate.Confidence,
			DedupeKey:  candidate.DedupeKey,
		})
		if err != nil {
			return fmt.Errorf("memory: promote write: %w", err)
		}
	}

	if candidate.ConversationID != "" {
		err = s.convStore.AppendEvent(ctx, model.Event{
			ID:             memGenerateID(),
			ConversationID: candidate.ConversationID,
			Kind:           "memory_promoted",
			PayloadJSON: []byte(fmt.Sprintf(
				`{"memory_id":"%s","project_id":"%s","agent_id":"%s","scope":"%s","dedupe_key":"%s"}`,
				id, candidate.ProjectID, candidate.AgentID, candidate.Scope, candidate.DedupeKey,
			)),
		})
		if err != nil {
			return fmt.Errorf("memory: promote event: %w", err)
		}
	}

	return nil
}
