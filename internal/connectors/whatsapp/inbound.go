package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	controlconnector "github.com/canhta/gistclaw/internal/connectors/control"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

type ConnectorRuntime interface {
	ReceiveInboundMessage(ctx context.Context, req runtime.InboundMessageCommand) (model.Run, error)
	InspectConversation(ctx context.Context, key conversations.ConversationKey) (runtime.ConversationStatus, error)
	ResetConversation(ctx context.Context, key conversations.ConversationKey) (runtime.ConversationResetOutcome, error)
}

type MessageSender interface {
	Send(ctx context.Context, chatID, text string) error
}

type WebhookHandler struct {
	verifyToken   string
	defaultAgent  string
	workspaceRoot string
	rt            ConnectorRuntime
	sender        MessageSender
	commands      *controlconnector.Dispatcher
	health        *HealthState
}

func NewWebhookHandler(
	verifyToken string,
	defaultAgent string,
	workspaceRoot string,
	rt ConnectorRuntime,
	sender MessageSender,
	health *HealthState,
) *WebhookHandler {
	if health == nil {
		health = NewHealthState(nil)
	}
	return &WebhookHandler{
		verifyToken:   verifyToken,
		defaultAgent:  defaultAgent,
		workspaceRoot: workspaceRoot,
		rt:            rt,
		sender:        sender,
		commands:      controlconnector.NewDispatcher(rt),
		health:        health,
	}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleVerify(w, r)
	case http.MethodPost:
		h.handleMessage(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *WebhookHandler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("hub.mode") != "subscribe" || r.URL.Query().Get("hub.verify_token") != h.verifyToken {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(r.URL.Query().Get("hub.challenge")))
}

func (h *WebhookHandler) handleMessage(w http.ResponseWriter, r *http.Request) {
	if h.rt == nil {
		http.Error(w, "runtime not configured", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	envelopes, err := NormalizeWebhookPayload(body)
	if err != nil {
		http.Error(w, "invalid webhook payload", http.StatusBadRequest)
		return
	}

	for _, env := range envelopes {
		err := h.handleEnvelope(r.Context(), env)
		if err != nil {
			http.Error(w, fmt.Sprintf("dispatch inbound message: %v", err), http.StatusInternalServerError)
			return
		}
	}
	h.health.markWebhook(time.Now().UTC())

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *WebhookHandler) handleEnvelope(ctx context.Context, env model.Envelope) error {
	reply, handled, err := h.commands.Dispatch(ctx, env)
	if err != nil {
		return err
	}
	if handled {
		if h.sender == nil {
			return fmt.Errorf("whatsapp: native reply sender not configured")
		}
		return h.sender.Send(ctx, env.ConversationID, reply)
	}

	_, err = h.rt.ReceiveInboundMessage(ctx, runtime.InboundMessageCommand{
		ConversationKey: conversations.ConversationKey{
			ConnectorID: env.ConnectorID,
			AccountID:   env.AccountID,
			ExternalID:  env.ConversationID,
			ThreadID:    env.ThreadID,
		},
		FrontAgentID:    h.defaultAgent,
		Body:            env.Text,
		SourceMessageID: env.MessageID,
		CWD:             h.workspaceRoot,
	})
	return err
}

type webhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
				Messages []struct {
					From      string `json:"from"`
					ID        string `json:"id"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

func NormalizeWebhookPayload(body []byte) ([]model.Envelope, error) {
	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("whatsapp: decode payload: %w", err)
	}

	envelopes := make([]model.Envelope, 0)
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			for _, msg := range change.Value.Messages {
				if msg.Type != "text" {
					continue
				}
				text := strings.TrimSpace(msg.Text.Body)
				if text == "" {
					continue
				}
				envelopes = append(envelopes, model.Envelope{
					ConnectorID:    "whatsapp",
					AccountID:      change.Value.Metadata.PhoneNumberID,
					ActorID:        msg.From,
					ConversationID: msg.From,
					ThreadID:       "main",
					MessageID:      msg.ID,
					Text:           text,
					ReceivedAt:     time.Now().UTC(),
				})
			}
		}
	}
	return envelopes, nil
}
