package conversations

import "strings"

type ConversationKey struct {
	ConnectorID string
	AccountID   string
	ExternalID  string
	ThreadID    string
	ProjectID   string
}

func (k ConversationKey) Normalize() string {
	thread := normalizeThreadID(k.ThreadID)

	escape := func(s string) string {
		return strings.ReplaceAll(s, ":", "%3A")
	}

	parts := []string{
		escape(k.ConnectorID),
		escape(k.AccountID),
		escape(k.ExternalID),
		escape(thread),
	}
	if k.ProjectID != "" {
		parts = append(parts, escape(k.ProjectID))
	}

	return strings.Join(parts, ":")
}

func normalizeThreadID(threadID string) string {
	if threadID == "" {
		return LocalDefaultThreadID
	}
	return threadID
}
