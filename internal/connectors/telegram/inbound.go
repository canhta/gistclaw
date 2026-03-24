package telegram

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

// recognizedPrefixes are message patterns accepted as task submissions.
// A message is accepted if it does not look like an unrecognized slash command
// (i.e. it either has no leading slash, or it uses a recognized slash command).
var recognizedCommands = map[string]bool{
	"/start": true,
	"/help":  true,
	"/run":   true,
	"/task":  true,
}

// NormalizeUpdate converts a raw Telegram Update into a model.Envelope.
//
// Returns an error with reason "DM only" if the chat is not private.
// Returns an error with reason "unrecognized command" if the message looks
// like a slash command that is not in the allowed set.
func NormalizeUpdate(upd Update) (model.Envelope, error) {
	if upd.Message == nil {
		return model.Envelope{}, fmt.Errorf("telegram: update has no message")
	}
	msg := upd.Message

	if msg.Chat.Type != "private" {
		return model.Envelope{}, fmt.Errorf("telegram: DM only — chat type %q is not allowed", msg.Chat.Type)
	}

	text := strings.TrimSpace(msg.Text)
	if strings.HasPrefix(text, "/") {
		// Extract the command word (up to first space).
		cmd := strings.SplitN(text, " ", 2)[0]
		if !recognizedCommands[cmd] {
			return model.Envelope{}, fmt.Errorf("telegram: unrecognized command %q", cmd)
		}
	}

	return model.Envelope{
		ConnectorID:    "telegram",
		AccountID:      strconv.FormatInt(msg.Chat.ID, 10),
		ActorID:        strconv.FormatInt(msg.Chat.ID, 10),
		ConversationID: strconv.FormatInt(msg.Chat.ID, 10),
		ThreadID:       "main",
		MessageID:      strconv.FormatInt(msg.MessageID, 10),
		Text:           text,
		ReceivedAt:     time.Now().UTC(),
	}, nil
}
