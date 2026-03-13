// internal/hitl/service.go
package hitl

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/store"
)

// QuestionReplier posts the collected answers back to the agent API.
// Implemented by opencode.Service; injected into hitl.Service at construction.
// This keeps hitl.Service decoupled from the OpenCode HTTP client.
type QuestionReplier interface {
	// ReplyQuestion posts {"answers": answers} to /question/:id/reply on the agent API.
	// id is the full question request ID (e.g. "question_<ulid>").
	// answers is one []string per question, each containing the selected/typed labels.
	ReplyQuestion(ctx context.Context, id string, answers [][]string) error
}

// Approver is the interface implemented by Service.
// Called by opencode.Service and claudecode.Service.
type Approver interface {
	// RequestPermission registers a permission request, sends a keyboard to the operator,
	// and returns immediately (non-blocking). The caller blocks on req.DecisionCh.
	RequestPermission(ctx context.Context, req PermissionRequest) error
	// RequestQuestion sends each question sequentially, collects user answers, then
	// calls QuestionReplier.ReplyQuestion with all answers and returns its error.
	// Design §7: returns only error; hitl.Service owns the reply POST (Flow G §5).
	RequestQuestion(ctx context.Context, req QuestionRequest) error
}

// pendingItem stores the in-flight state for a PermissionRequest.
type pendingItem struct {
	decisionCh chan<- HITLDecision
}

// questionWaiterItem stores a single pending question answer slot.
// The Question is stored so the callback handler can resolve option index → label.
type questionWaiterItem struct {
	answerCh chan string // receives exactly one answer string (the resolved label)
	question Question    // original question; used to map opt:<n> → label
}

// Service implements Approver. It is started by app.Run via withRestart.
type Service struct {
	ch      channel.Channel
	store   *store.Store
	tuning  config.Tuning
	replier QuestionReplier // posts /question/:id/reply to the agent API

	// pending tracks in-flight PermissionRequests keyed by ID.
	pending sync.Map

	// questionWaiters is a sync.Map[string, questionWaiterItem] keyed by question request ID.
	// Each entry is written by RequestQuestion and consumed by the event loop callback.
	questionWaiters sync.Map
}

// NewService creates a new HITL service.
// replier is called by RequestQuestion to POST answers to the agent API after all
// questions are answered. Pass nil only in tests that do not exercise RequestQuestion.
func NewService(ch channel.Channel, s *store.Store, tuning config.Tuning, replier QuestionReplier) *Service {
	return &Service{ch: ch, store: s, tuning: tuning, replier: replier}
}

// Run is the main event loop. It:
//  1. Auto-rejects all hitl_pending records with status "pending" (from a previous run).
//  2. Calls ch.Receive to get the inbound message stream.
//  3. Dispatches callback messages to the appropriate registered handler.
//
// Blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	// Startup auto-reject: update stale pending records to "auto_rejected".
	if err := s.startupAutoReject(ctx); err != nil {
		log.Warn().Err(err).Msg("hitl: startup auto-reject failed")
	}

	msgs, err := s.ch.Receive(ctx)
	if err != nil {
		return fmt.Errorf("hitl: channel.Receive: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			if msg.CallbackData != "" && strings.HasPrefix(msg.CallbackData, "hitl:") {
				s.dispatchCallback(ctx, msg)
			}
		}
	}
}

// DrainPending sends HITLDecision{Allow: false} on all in-flight PermissionRequest
// channels. Called by app.Run when hitl.Service permanently fails.
func (s *Service) DrainPending() {
	s.pending.Range(func(key, val any) bool {
		item := val.(pendingItem)
		select {
		case item.decisionCh <- HITLDecision{Allow: false}:
		default:
			// Channel already has a value or was closed; skip.
		}
		s.pending.Delete(key)
		return true
	})
}

// RequestPermission writes a hitl_pending record, sends a keyboard, registers the
// DecisionCh in the in-memory map, and returns nil immediately (non-blocking).
func (s *Service) RequestPermission(ctx context.Context, req PermissionRequest) error {
	// Write to SQLite first (prevents race if user replies before registration).
	if err := s.store.InsertHITLPending(req.ID, req.SessionID, req.Permission); err != nil {
		return fmt.Errorf("hitl: insert pending: %w", err)
	}

	// Register the decision channel.
	s.pending.Store(req.ID, pendingItem{decisionCh: req.DecisionCh})

	// Send keyboard asynchronously so RequestPermission returns immediately.
	go func() {
		payload := PermissionKeyboard(req.ID, req.Permission, req.Patterns)
		if err := s.ch.SendKeyboard(ctx, req.ChatID, payload); err != nil {
			log.Warn().Err(err).Str("id", req.ID).Msg("hitl: failed to send permission keyboard")
		}

		// Schedule a reminder before timeout.
		reminderDelay := s.tuning.HITLTimeout - s.tuning.HITLReminderBefore
		if reminderDelay > 0 {
			select {
			case <-time.After(reminderDelay):
				// Only send reminder if still pending.
				if _, ok := s.pending.Load(req.ID); ok {
					msg := fmt.Sprintf("⏰ Approval still pending for: %s", req.ID)
					_ = s.ch.SendMessage(ctx, req.ChatID, msg)
				}
			case <-ctx.Done():
			}
		}
	}()

	return nil
}

// RequestQuestion processes questions sequentially.
// For each question: sends a keyboard, registers a waiter, blocks until the user
// replies or timeout. The waiter stores the Question so the callback handler can
// resolve an option index into the actual label string.
// After all questions are answered, calls s.replier.ReplyQuestion with collected
// answers and returns its error.
func (s *Service) RequestQuestion(ctx context.Context, req QuestionRequest) error {
	allAnswers := make([][]string, len(req.Questions))

	for i, q := range req.Questions {
		payload := QuestionKeyboard(req.ID, q)
		if err := s.ch.SendKeyboard(ctx, req.ChatID, payload); err != nil {
			log.Warn().Err(err).Str("id", req.ID).Int("q", i).Msg("hitl: failed to send question keyboard")
		}

		// Register a waiter for this question. The event loop resolves opt:<n> to
		// the label using item.question.Options[n].Label.
		answerCh := make(chan string, 1)
		s.questionWaiters.Store(req.ID, questionWaiterItem{answerCh: answerCh, question: q})

		var answer string
		select {
		case answer = <-answerCh:
		case <-time.After(s.tuning.HITLTimeout):
			log.Warn().Str("id", req.ID).Int("q", i).Msg("hitl: question timed out; using empty answer")
			answer = ""
		case <-ctx.Done():
			return ctx.Err()
		}

		s.questionWaiters.Delete(req.ID)

		if answer == "" {
			allAnswers[i] = []string{}
		} else {
			allAnswers[i] = []string{answer}
		}
	}

	// POST all answers to the agent API in a single call (design Flow G §5).
	if s.replier != nil {
		if err := s.replier.ReplyQuestion(ctx, req.ID, allAnswers); err != nil {
			return fmt.Errorf("hitl: reply question %s: %w", req.ID, err)
		}
	}
	return nil
}

// dispatchCallback handles inbound callback messages whose data starts with "hitl:".
//
// Permission callbacks: "hitl:<id>:once|always|reject|stop"
// Question callbacks:   "hitl:<id>:opt:<n>" or "hitl:<id>:custom"
func (s *Service) dispatchCallback(ctx context.Context, msg channel.InboundMessage) {
	// Parse "hitl:<id>:<action>" or "hitl:<id>:opt:<n>"
	parts := strings.SplitN(msg.CallbackData, ":", 4) // ["hitl", "<id>", "<action>", optional-index]
	if len(parts) < 3 {
		log.Warn().Str("data", msg.CallbackData).Msg("hitl: malformed callback data")
		return
	}
	id := parts[1]
	action := parts[2]

	// Check if it's a permission callback.
	if val, ok := s.pending.Load(id); ok {
		item := val.(pendingItem)
		var decision HITLDecision
		switch action {
		case "once":
			decision = HITLDecision{Allow: true, Always: false}
		case "always":
			decision = HITLDecision{Allow: true, Always: true}
		case "reject":
			decision = HITLDecision{Allow: false}
		case "stop":
			decision = HITLDecision{Allow: false, Stop: true}
		default:
			log.Warn().Str("action", action).Msg("hitl: unknown permission action")
			return
		}

		select {
		case item.decisionCh <- decision:
		default:
		}
		s.pending.Delete(id)

		if err := s.store.ResolveHITL(id, "resolved"); err != nil {
			log.Warn().Err(err).Str("id", id).Msg("hitl: failed to resolve hitl_pending in SQLite")
		}
		return
	}

	// Check if it's a question callback.
	if val, ok := s.questionWaiters.Load(id); ok {
		item := val.(questionWaiterItem)
		var answer string
		switch action {
		case "opt":
			// "hitl:<id>:opt:<n>" — resolve the option index to the actual label.
			if len(parts) == 4 {
				idxStr := parts[3]
				var idx int
				if n, _ := fmt.Sscanf(idxStr, "%d", &idx); n != 1 {
					log.Warn().Str("id", id).Str("idx", idxStr).Msg("hitl: opt index not parseable; using empty")
				} else if idx >= 0 && idx < len(item.question.Options) {
					answer = item.question.Options[idx].Label
				} else {
					log.Warn().Str("id", id).Str("idx", idxStr).Msg("hitl: opt index out of range; using empty")
				}
			}
		case "custom":
			// User wants to type a custom answer. For v1: return empty string.
			answer = ""
		default:
			answer = action
		}
		select {
		case item.answerCh <- answer:
		default:
		}
		return
	}

	log.Warn().Str("id", id).Str("action", action).Msg("hitl: received callback for unknown request")
}

// Resolve looks up a pending PermissionRequest by id and sends the appropriate
// HITLDecision based on action ("once", "always", "reject", "stop").
// Returns an error if id is not found or action is unknown.
func (s *Service) Resolve(id string, action string) error {
	val, ok := s.pending.Load(id)
	if !ok {
		return fmt.Errorf("hitl: Resolve: id %q not found", id)
	}
	item := val.(pendingItem)

	var decision HITLDecision
	switch action {
	case "once":
		decision = HITLDecision{Allow: true, Always: false}
	case "always":
		decision = HITLDecision{Allow: true, Always: true}
	case "reject":
		decision = HITLDecision{Allow: false}
	case "stop":
		decision = HITLDecision{Allow: false, Stop: true}
	default:
		return fmt.Errorf("hitl: Resolve: unknown action %q", action)
	}

	select {
	case item.decisionCh <- decision:
	default:
	}
	s.pending.Delete(id)
	if err := s.store.ResolveHITL(id, "resolved"); err != nil {
		log.Warn().Err(err).Str("id", id).Msg("hitl: Resolve: failed to update SQLite")
	}
	return nil
}

// startupAutoReject marks all hitl_pending records with status "pending" as
// "auto_rejected" in SQLite. On restart, the in-memory sync.Map is empty so there
// are no channels to notify — only SQLite is updated.
func (s *Service) startupAutoReject(ctx context.Context) error {
	pending, err := s.store.ListPendingHITL()
	if err != nil {
		return fmt.Errorf("hitl: list pending on startup: %w", err)
	}

	for _, rec := range pending {
		if err := s.store.ResolveHITL(rec.ID, "auto_rejected"); err != nil {
			log.Warn().Err(err).Str("id", rec.ID).Msg("hitl: failed to auto-reject stale pending record")
			continue
		}
		log.Info().Str("id", rec.ID).Msg("hitl: auto-rejected stale pending record on startup")
	}
	return nil
}
