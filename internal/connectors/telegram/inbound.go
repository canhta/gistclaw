package telegram

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
)

// NormalizeUpdate converts a raw Telegram Update into a model.Envelope.
//
// Returns an error with reason "DM only" if the chat is not private.
func NormalizeUpdate(upd Update) (model.Envelope, error) {
	if upd.Message == nil {
		return model.Envelope{}, fmt.Errorf("telegram: update has no message")
	}
	msg := upd.Message

	if msg.Chat.Type != "private" {
		return model.Envelope{}, fmt.Errorf("telegram: DM only — chat type %q is not allowed", msg.Chat.Type)
	}

	text := normalizeMessageText(msg.Text)

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

func normalizeMessageText(raw string) string {
	return strings.TrimSpace(raw)
}
