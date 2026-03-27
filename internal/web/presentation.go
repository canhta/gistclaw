package web

import (
	"fmt"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

func formatWebTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format("2006-01-02 15:04:05 UTC")
}

func formatOptionalWebTimestamp(ts *time.Time) string {
	if ts == nil {
		return ""
	}
	return formatWebTimestamp(*ts)
}

func humanizeWebLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	words := strings.Fields(strings.ReplaceAll(raw, "_", " "))
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
	}
	return strings.Join(words, " ")
}

func sessionRoleLabel(role model.SessionRole) string {
	switch role {
	case model.SessionRoleFront:
		return "Lead agent"
	case model.SessionRoleWorker:
		return "Specialist agent"
	default:
		label := humanizeWebLabel(string(role))
		if label == "" {
			return ""
		}
		return label + " agent"
	}
}

func sessionRoleSummaryLabel(role model.SessionRole) string {
	switch role {
	case model.SessionRoleFront:
		return "Lead agent"
	case model.SessionRoleWorker:
		return "Specialist agent"
	default:
		return sessionRoleLabel(role)
	}
}

func sessionMessageKindLabel(kind model.SessionMessageKind) string {
	switch kind {
	case model.MessageAnnounce:
		return "Note"
	case model.MessageAgentSend:
		return "Agent reply"
	default:
		return humanizeWebLabel(string(kind))
	}
}

func sessionSenderLabel(senderSessionID string) string {
	if strings.TrimSpace(senderSessionID) == "" {
		return "You / GistClaw"
	}
	return senderSessionID
}

func attemptLabel(attempts int) string {
	if attempts == 1 {
		return "1 attempt"
	}
	return fmt.Sprintf("%d attempts", attempts)
}
