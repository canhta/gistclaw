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
	if upd.CallbackQuery != nil {
		return normalizeCallbackQuery(*upd.CallbackQuery)
	}
	if upd.Message == nil {
		return model.Envelope{}, fmt.Errorf("telegram: update has no message")
	}
	return normalizeMessage(*upd.Message)
}

func normalizeMessage(msg Message) (model.Envelope, error) {
	if msg.Chat.Type != "private" {
		return model.Envelope{}, fmt.Errorf("telegram: DM only — chat type %q is not allowed", msg.Chat.Type)
	}

	text := normalizeMessageText(msg.Text)
	metadata := make(map[string]string)
	if languageHint := strings.TrimSpace(msg.From.LanguageCode); languageHint != "" {
		metadata["language_hint"] = languageHint
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
		Metadata:       metadata,
	}, nil
}

func normalizeCallbackQuery(cbq CallbackQuery) (model.Envelope, error) {
	if cbq.Message == nil {
		return model.Envelope{}, fmt.Errorf("telegram: callback query has no message")
	}
	msg := cbq.Message
	if msg.Chat.Type != "private" {
		return model.Envelope{}, fmt.Errorf("telegram: DM only — chat type %q is not allowed", msg.Chat.Type)
	}

	metadata := make(map[string]string)
	if languageHint := strings.TrimSpace(cbq.From.LanguageCode); languageHint != "" {
		metadata["language_hint"] = languageHint
	}

	return model.Envelope{
		ConnectorID:    "telegram",
		AccountID:      strconv.FormatInt(msg.Chat.ID, 10),
		ActorID:        strconv.FormatInt(cbq.From.ID, 10),
		ConversationID: strconv.FormatInt(msg.Chat.ID, 10),
		ThreadID:       "main",
		MessageID:      strings.TrimSpace(cbq.ID),
		Text:           normalizeMessageText(cbq.Data),
		ReceivedAt:     time.Now().UTC(),
		Metadata:       metadata,
	}, nil
}

func normalizeMessageText(raw string) string {
	return strings.TrimSpace(raw)
}
