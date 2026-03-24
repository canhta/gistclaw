package telegram

import (
	"fmt"
	"strconv"
	"strings"
)

// Envelope is the normalized form of an inbound Telegram DM.
type Envelope struct {
	ConnectorID string
	AccountID   string
	ExternalID  string
	ThreadID    string
	Body        string
}

// recognizedPrefixes are message patterns accepted as task submissions.
// A message is accepted if it does not look like an unrecognized slash command
// (i.e. it either has no leading slash, or it uses a recognized slash command).
var recognizedCommands = map[string]bool{
	"/start": true,
	"/help":  true,
	"/run":   true,
	"/task":  true,
}

// NormalizeUpdate converts a raw Telegram Update into an internal Envelope.
//
// Returns an error with reason "DM only" if the chat is not private.
// Returns an error with reason "unrecognized command" if the message looks
// like a slash command that is not in the allowed set.
func NormalizeUpdate(upd Update) (Envelope, error) {
	if upd.Message == nil {
		return Envelope{}, fmt.Errorf("telegram: update has no message")
	}
	msg := upd.Message

	if msg.Chat.Type != "private" {
		return Envelope{}, fmt.Errorf("telegram: DM only — chat type %q is not allowed", msg.Chat.Type)
	}

	text := strings.TrimSpace(msg.Text)
	if strings.HasPrefix(text, "/") {
		// Extract the command word (up to first space).
		cmd := strings.SplitN(text, " ", 2)[0]
		if !recognizedCommands[cmd] {
			return Envelope{}, fmt.Errorf("telegram: unrecognized command %q", cmd)
		}
	}

	return Envelope{
		ConnectorID: "telegram",
		AccountID:   strconv.FormatInt(msg.Chat.ID, 10),
		ExternalID:  strconv.FormatInt(msg.MessageID, 10),
		ThreadID:    "main",
		Body:        text,
	}, nil
}
