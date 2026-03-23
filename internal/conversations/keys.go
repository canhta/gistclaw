package conversations

import (
	"fmt"
	"strings"
)

type ConversationKey struct {
	ConnectorID string
	AccountID   string
	ExternalID  string
	ThreadID    string
}

func (k ConversationKey) Normalize() string {
	thread := k.ThreadID
	if thread == "" {
		thread = "main"
	}

	escape := func(s string) string {
		return strings.ReplaceAll(s, ":", "%3A")
	}

	return fmt.Sprintf("%s:%s:%s:%s",
		escape(k.ConnectorID),
		escape(k.AccountID),
		escape(k.ExternalID),
		escape(thread),
	)
}
