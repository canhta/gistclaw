package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
)

type ConversationGateReplyCommand struct {
	ConversationKey conversations.ConversationKey
	Body            string
	SourceMessageID string
	LanguageHint    string
	ProjectID       string
	CWD             string
}

type ConversationGateReplyOutcome struct {
	Handled  bool
	GateID   string
	Decision string
}

type gateApprovalCommand struct {
	Handled  bool
	Decision string
}

type gateResolverResult struct {
	Action     string `json:"action"`
	Confidence string `json:"confidence"`
	ReplyText  string `json:"reply_text"`
}

func defaultGateClarificationReply() string {
	return "I couldn't tell whether you want to approve or deny that. Reply yes/no, approve/deny, or answer in your language."
}

func (r *Runtime) HandleConversationGateReply(ctx context.Context, cmd ConversationGateReplyCommand) (ConversationGateReplyOutcome, error) {
	if strings.TrimSpace(cmd.Body) == "" {
		return ConversationGateReplyOutcome{}, nil
	}

	threadID := normalizeThreadID(cmd.ConversationKey.ThreadID)
	cmd.ConversationKey.ThreadID = threadID
	gate, err := r.resolvePendingConversationGate(ctx, cmd.ConversationKey, cmd.ProjectID, cmd.CWD)
	if err == sql.ErrNoRows {
		return ConversationGateReplyOutcome{}, nil
	}
	if err != nil {
		return ConversationGateReplyOutcome{}, fmt.Errorf("resolve conversation gate: %w", err)
	}

	if cmd.SourceMessageID != "" {
		if _, err := r.loadInboundReceiptRun(
			ctx,
			gate.ConversationID,
			cmd.ConversationKey.ConnectorID,
			cmd.ConversationKey.AccountID,
			threadID,
			cmd.SourceMessageID,
		); err == nil {
			return ConversationGateReplyOutcome{Handled: true}, nil
		} else if err != nil && err != sql.ErrNoRows {
			return ConversationGateReplyOutcome{}, err
		}
	}

	if _, err := r.appendInboundGateMessage(ctx, gate.ConversationID, gate.RunID, gate.SessionID, cmd.ConversationKey, threadID, cmd.SourceMessageID, cmd.LanguageHint, cmd.Body); err != nil {
		return ConversationGateReplyOutcome{}, err
	}

	if gate.Kind != model.ConversationGateApproval {
		return ConversationGateReplyOutcome{Handled: true, GateID: gate.ID}, nil
	}

	commandDecision, err := parseApprovalCommandReply(strings.TrimSpace(cmd.Body), gate)
	if err != nil {
		if err := r.appendGateAssistantMessage(ctx, gate, err.Error()); err != nil {
			return ConversationGateReplyOutcome{}, err
		}
		return ConversationGateReplyOutcome{Handled: true, GateID: gate.ID}, nil
	}
	if commandDecision.Handled {
		if err := r.resolveGateApprovalAsync(ctx, gate, commandDecision.Decision); err != nil {
			return ConversationGateReplyOutcome{}, err
		}
		return ConversationGateReplyOutcome{Handled: true, GateID: gate.ID, Decision: commandDecision.Decision}, nil
	}

	resolution, err := r.resolveGateReplyWithModel(ctx, gate, cmd.Body, cmd.LanguageHint)
	if err != nil {
		return ConversationGateReplyOutcome{}, err
	}

	action := strings.ToLower(strings.TrimSpace(resolution.Action))
	confidence := strings.ToLower(strings.TrimSpace(resolution.Confidence))
	switch action {
	case "approve", "approved", "allow":
		if confidence == "high" {
			if err := r.resolveGateApprovalAsync(ctx, gate, "approved"); err != nil {
				return ConversationGateReplyOutcome{}, err
			}
			return ConversationGateReplyOutcome{Handled: true, GateID: gate.ID, Decision: "approved"}, nil
		}
	case "deny", "denied", "reject", "skip":
		if confidence == "high" {
			if err := r.resolveGateApprovalAsync(ctx, gate, "denied"); err != nil {
				return ConversationGateReplyOutcome{}, err
			}
			return ConversationGateReplyOutcome{Handled: true, GateID: gate.ID, Decision: "denied"}, nil
		}
	}

	replyText := strings.TrimSpace(resolution.ReplyText)
	if replyText == "" {
		replyText = defaultGateClarificationReply()
	}
	if err := r.appendGateAssistantMessage(ctx, gate, replyText); err != nil {
		return ConversationGateReplyOutcome{}, err
	}
	return ConversationGateReplyOutcome{Handled: true, GateID: gate.ID}, nil
}

func (r *Runtime) resolvePendingConversationGate(
	ctx context.Context,
	key conversations.ConversationKey,
	projectID string,
	cwd string,
) (model.ConversationGate, error) {
	keys := make([]conversations.ConversationKey, 0, 2)
	keys = append(keys, key)

	if scopedKey, _, err := r.scopeConversationKey(ctx, key, projectID, cwd); err == nil && scopedKey.Normalize() != key.Normalize() {
		keys = append(keys, scopedKey)
	}

	seen := make(map[string]bool, len(keys))
	for _, candidate := range keys {
		normalized := candidate.Normalize()
		if seen[normalized] {
			continue
		}
		seen[normalized] = true

		conv, found, err := r.convStore.Find(ctx, candidate)
		if err != nil {
			return model.ConversationGate{}, err
		}
		if !found {
			continue
		}
		gate, err := r.loadActiveConversationGate(ctx, conv.ID)
		if err == nil {
			return gate, nil
		}
		if err != sql.ErrNoRows {
			return model.ConversationGate{}, err
		}
	}

	return r.loadActiveConversationGateByRoute(
		ctx,
		key.ConnectorID,
		key.AccountID,
		key.ExternalID,
		normalizeThreadID(key.ThreadID),
	)
}

func (r *Runtime) loadActiveConversationGate(ctx context.Context, conversationID string) (model.ConversationGate, error) {
	var gate model.ConversationGate
	err := r.store.RawDB().QueryRowContext(ctx,
		`SELECT id, conversation_id, run_id, session_id, kind, status, COALESCE(approval_id, ''),
		        title, body, options_json, metadata_json, COALESCE(language_hint, ''), created_at, resolved_at
		 FROM conversation_gates
		 WHERE conversation_id = ? AND status = 'pending'
		 ORDER BY created_at ASC, id ASC
		 LIMIT 1`,
		conversationID,
	).Scan(
		&gate.ID,
		&gate.ConversationID,
		&gate.RunID,
		&gate.SessionID,
		&gate.Kind,
		&gate.Status,
		&gate.ApprovalID,
		&gate.Title,
		&gate.Body,
		&gate.OptionsJSON,
		&gate.MetadataJSON,
		&gate.LanguageHint,
		&gate.CreatedAt,
		&gate.ResolvedAt,
	)
	if err != nil {
		return model.ConversationGate{}, err
	}
	return gate, nil
}

func (r *Runtime) loadActiveConversationGateByRoute(
	ctx context.Context,
	connectorID string,
	accountID string,
	externalID string,
	threadID string,
) (model.ConversationGate, error) {
	var gate model.ConversationGate
	err := r.store.RawDB().QueryRowContext(ctx,
		`SELECT gate.id, gate.conversation_id, gate.run_id, gate.session_id, gate.kind, gate.status,
		        COALESCE(gate.approval_id, ''), gate.title, gate.body, gate.options_json, gate.metadata_json,
		        COALESCE(gate.language_hint, ''), gate.created_at, gate.resolved_at
		 FROM conversation_gates gate
		 JOIN session_bindings bind
		   ON bind.conversation_id = gate.conversation_id
		  AND bind.session_id = gate.session_id
		 WHERE bind.connector_id = ?
		   AND bind.account_id = ?
		   AND bind.external_id = ?
		   AND bind.thread_id = ?
		   AND bind.status = 'active'
		   AND gate.status = 'pending'
		 ORDER BY gate.created_at ASC, gate.id ASC
		 LIMIT 1`,
		connectorID,
		accountID,
		externalID,
		threadID,
	).Scan(
		&gate.ID,
		&gate.ConversationID,
		&gate.RunID,
		&gate.SessionID,
		&gate.Kind,
		&gate.Status,
		&gate.ApprovalID,
		&gate.Title,
		&gate.Body,
		&gate.OptionsJSON,
		&gate.MetadataJSON,
		&gate.LanguageHint,
		&gate.CreatedAt,
		&gate.ResolvedAt,
	)
	if err != nil {
		return model.ConversationGate{}, err
	}
	return gate, nil
}

func parseApprovalCommandReply(body string, gate model.ConversationGate) (gateApprovalCommand, error) {
	if !strings.HasPrefix(strings.ToLower(body), "/approve") {
		return gateApprovalCommand{}, nil
	}
	tokens := strings.Fields(body)
	if len(tokens) < 3 {
		return gateApprovalCommand{Handled: true}, fmt.Errorf("usage: /approve %s allow-once|allow-always|deny", gate.ApprovalID)
	}
	approvalID := strings.TrimSpace(tokens[1])
	if approvalID != gate.ApprovalID && approvalID != gate.ID {
		return gateApprovalCommand{Handled: true}, fmt.Errorf("approval ID does not match the pending request")
	}
	decision := strings.ToLower(strings.TrimSpace(tokens[2]))
	switch decision {
	case "allow", "once", "allow-once", "allowonce", "allow-always", "allowalways", "always":
		return gateApprovalCommand{Handled: true, Decision: "approved"}, nil
	case "deny", "denied", "reject":
		return gateApprovalCommand{Handled: true, Decision: "denied"}, nil
	default:
		return gateApprovalCommand{Handled: true}, fmt.Errorf("usage: /approve %s allow-once|allow-always|deny", gate.ApprovalID)
	}
}

func (r *Runtime) resolveGateReplyWithModel(ctx context.Context, gate model.ConversationGate, body string, replyLanguageHint string) (gateResolverResult, error) {
	instructionsParts := []string{
		"You resolve one pending operator approval gate.",
		"Return JSON only with keys: action, confidence, reply_text.",
		"Valid actions: approve, deny, clarify.",
		"Use action=clarify unless the user's intent is clearly about this pending gate.",
		"Only return confidence=high when the decision is unambiguous.",
		"Interpret approvals and denials in any language, including short replies and mixed-language replies.",
		"If action=clarify, write reply_text in the user's language when clear. If the language is unclear, keep the clarification short and understandable.",
		fmt.Sprintf("Gate title: %s", gate.Title),
		fmt.Sprintf("Gate body: %s", gate.Body),
		fmt.Sprintf("Pending approval id: %s", gate.ApprovalID),
		fmt.Sprintf("User reply: %s", body),
	}
	if hint := strings.TrimSpace(replyLanguageHint); hint != "" {
		instructionsParts = append(instructionsParts, fmt.Sprintf("Reply language hint: %s", hint))
	} else if hint := strings.TrimSpace(gate.LanguageHint); hint != "" {
		instructionsParts = append(instructionsParts, fmt.Sprintf("Conversation language hint: %s", hint))
	}
	instructions := strings.Join(instructionsParts, "\n")
	result, err := r.provider.Generate(ctx, GenerateRequest{
		Instructions: instructions,
		ToolSpecs:    nil,
		MaxTokens:    128,
	}, nil)
	if err != nil {
		return gateResolverResult{}, err
	}
	var parsed gateResolverResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Content)), &parsed); err != nil {
		return gateResolverResult{
			Action:     "clarify",
			Confidence: "low",
			ReplyText:  defaultGateClarificationReply(),
		}, nil
	}
	return parsed, nil
}

func (r *Runtime) resolveGateApprovalAsync(ctx context.Context, gate model.ConversationGate, decision string) error {
	if gate.ApprovalID == "" {
		return fmt.Errorf("runtime: gate %s has no approval id", gate.ID)
	}
	return r.ResolveApprovalAsync(ctx, gate.ApprovalID, decision)
}

func (r *Runtime) appendGateAssistantMessage(ctx context.Context, gate model.ConversationGate, body string) error {
	if gate.SessionID == "" || strings.TrimSpace(body) == "" {
		return nil
	}
	messageID, err := r.appendSessionMessage(
		ctx,
		gate.ConversationID,
		gate.RunID,
		gate.SessionID,
		gate.SessionID,
		model.MessageAssistant,
		body,
		model.SessionMessageProvenance{
			Kind:        model.MessageProvenanceAssistantTurn,
			SourceRunID: gate.RunID,
		},
	)
	if err != nil {
		return err
	}
	return r.queueConversationOutboundIntent(ctx, gate.RunID, gate.ConversationID, gate.SessionID, messageID, body, nil)
}

func (r *Runtime) appendInboundGateMessage(
	ctx context.Context,
	conversationID string,
	runID string,
	sessionID string,
	key conversations.ConversationKey,
	threadID string,
	sourceMessageID string,
	languageHint string,
	body string,
) (string, error) {
	now := time.Now().UTC()
	messageID := generateID()
	events := make([]model.Event, 0, 2)
	messageEvent, err := newSessionMessageAddedEvent(
		conversationID,
		runID,
		sessionID,
		"",
		model.MessageUser,
		body,
		model.SessionMessageProvenance{
			Kind:              model.MessageProvenanceInbound,
			SourceConnectorID: key.ConnectorID,
			SourceThreadID:    threadID,
			SourceMessageID:   sourceMessageID,
			LanguageHint:      strings.TrimSpace(languageHint),
		},
		messageID,
		now,
	)
	if err != nil {
		return "", err
	}
	events = append(events, messageEvent)
	if sourceMessageID != "" {
		recordedEvent, err := newInboundMessageRecordedEvent(
			conversationID,
			runID,
			key.ConnectorID,
			key.AccountID,
			threadID,
			sourceMessageID,
			sessionID,
			messageID,
			now,
		)
		if err != nil {
			return "", err
		}
		events = append(events, recordedEvent)
	}
	if err := r.convStore.AppendEvents(ctx, events); err != nil {
		return "", err
	}
	return messageID, nil
}
