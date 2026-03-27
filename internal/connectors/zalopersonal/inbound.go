package zalopersonal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

type InboundMessageReceiver interface {
	ReceiveInboundMessage(ctx context.Context, req runtime.InboundMessageCommand) (model.Run, error)
}

type InboundDispatcher struct {
	rt             InboundMessageReceiver
	defaultAgentID string
}

type IncomingMessage struct {
	AccountID      string
	SenderID       string
	ConversationID string
	MessageID      string
	Text           string
	LanguageHint   string
	IsDirect       bool
	Mentioned      bool
}

type GroupPolicy struct {
	Enabled         bool
	Allowlist       map[string]bool
	MentionRequired bool
}

func NewInboundDispatcher(rt InboundMessageReceiver, defaultAgentID string) *InboundDispatcher {
	return &InboundDispatcher{rt: rt, defaultAgentID: defaultAgentID}
}

func NormalizeInboundMessage(msg IncomingMessage) (model.Envelope, error) {
	return NormalizeInboundMessageWithPolicy(msg, GroupPolicy{})
}

func NormalizeInboundMessageWithPolicy(msg IncomingMessage, policy GroupPolicy) (model.Envelope, error) {
	if !msg.IsDirect {
		if !policy.Enabled {
			return model.Envelope{}, fmt.Errorf("zalo personal inbound: DM only")
		}
		if len(policy.Allowlist) == 0 || !policy.Allowlist[strings.TrimSpace(msg.ConversationID)] {
			return model.Envelope{}, fmt.Errorf("zalo personal inbound: group not allowed")
		}
		if policy.MentionRequired && !msg.Mentioned {
			return model.Envelope{}, fmt.Errorf("zalo personal inbound: group mention required")
		}
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return model.Envelope{}, fmt.Errorf("zalo personal inbound: text is required")
	}

	env := model.Envelope{
		ConnectorID:    "zalo_personal",
		AccountID:      strings.TrimSpace(msg.AccountID),
		ActorID:        strings.TrimSpace(msg.SenderID),
		ConversationID: strings.TrimSpace(msg.ConversationID),
		ThreadID:       "main",
		MessageID:      strings.TrimSpace(msg.MessageID),
		Text:           text,
		ReceivedAt:     time.Now().UTC(),
	}
	if hint := strings.TrimSpace(msg.LanguageHint); hint != "" {
		env.Metadata = map[string]string{"language_hint": hint}
	}
	return env, nil
}

func (d *InboundDispatcher) Dispatch(ctx context.Context, env model.Envelope) error {
	if strings.TrimSpace(env.Text) == "" {
		return fmt.Errorf("zalo personal inbound: dispatch text is required")
	}

	_, err := d.rt.ReceiveInboundMessage(ctx, runtime.InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: env.ConnectorID,
			AccountID:   env.AccountID,
			ExternalID:  env.ConversationID,
			ThreadID:    env.ThreadID,
		},
		FrontAgentID:    d.defaultAgentID,
		Body:            env.Text,
		SourceMessageID: env.MessageID,
		LanguageHint:    env.Metadata["language_hint"],
	})
	if err != nil {
		return fmt.Errorf("zalo personal inbound: receive inbound message: %w", err)
	}
	return nil
}
