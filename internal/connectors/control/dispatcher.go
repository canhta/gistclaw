package control

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/i18n"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

var defaultCommandSpecs = []CommandSpec{
	{Name: "start", Description: "Show help and how to use the bot"},
	{Name: "help", Description: "Show the available commands"},
	{Name: "status", Description: "Show the latest status for this chat"},
	{Name: "reset", Description: "Clear the current chat history and temp state"},
}

var defaultRegistry = NewRegistry(defaultCommandSpecs...)

// TODO(m3): Add /cancel once inbound runs execute off their transport loops and
// own cancellable contexts that can interrupt an active provider turn.

type ConversationInspector interface {
	InspectConversation(ctx context.Context, key conversations.ConversationKey) (runtime.ConversationStatus, error)
	ResetConversation(ctx context.Context, key conversations.ConversationKey) (runtime.ConversationResetOutcome, error)
}

type Dispatcher struct {
	registry Registry
	statuses ConversationInspector
}

func NewDispatcher(statuses ConversationInspector) *Dispatcher {
	return &Dispatcher{
		registry: defaultRegistry,
		statuses: statuses,
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, env model.Envelope) (string, bool, error) {
	command, ok := d.registry.Parse(env.Text)
	if !ok {
		return "", false, nil
	}

	switch command.Name {
	case "start", "help":
		return helpText(env.Metadata["language_hint"]), true, nil
	case "status":
		status, err := d.statuses.InspectConversation(ctx, conversationKeyFromEnvelope(env))
		if err != nil {
			return "", false, fmt.Errorf("control: inspect conversation status: %w", err)
		}
		return formatConversationStatus(env.Metadata["language_hint"], status), true, nil
	case "reset":
		outcome, err := d.statuses.ResetConversation(ctx, conversationKeyFromEnvelope(env))
		if err != nil {
			return "", false, fmt.Errorf("control: reset conversation: %w", err)
		}
		return formatResetOutcome(env.Metadata["language_hint"], outcome), true, nil
	default:
		return "", false, nil
	}
}

func helpText(languageHint string) string {
	return strings.Join([]string{
		i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlHelpIntro, nil),
		"",
		i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlHelpHeader, nil),
		i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlHelpCommandHelp, nil),
		i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlHelpCommandStatus, nil),
		i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlHelpCommandReset, nil),
	}, "\n")
}

func conversationKeyFromEnvelope(env model.Envelope) conversations.ConversationKey {
	return conversations.ConversationKey{
		ConnectorID: env.ConnectorID,
		AccountID:   env.AccountID,
		ExternalID:  env.ConversationID,
		ThreadID:    env.ThreadID,
	}
}

func formatConversationStatus(languageHint string, status runtime.ConversationStatus) string {
	if !status.Exists {
		return strings.Join([]string{
			i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlStatusNoRuns, nil),
			i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlStatusNoRunsHint, nil),
		}, "\n")
	}

	lines := make([]string, 0, 4)
	if status.ActiveRun.ID != "" {
		lines = append(lines, i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlStatusActiveRun, map[string]string{
			"run_id":    displayRunID(status.ActiveRun.ID),
			"objective": displayObjective(languageHint, status.ActiveRun.Objective),
		}))
	} else {
		lines = append(lines, i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlStatusNoActiveRun, nil))
	}

	if status.LatestRootRun.ID != "" && status.LatestRootRun.ID != status.ActiveRun.ID {
		lines = append(lines, i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlStatusLastRun, map[string]string{
			"run_id":    displayRunID(status.LatestRootRun.ID),
			"status":    string(status.LatestRootRun.Status),
			"objective": displayObjective(languageHint, status.LatestRootRun.Objective),
		}))
	}

	if status.PendingApprovals == 1 {
		lines = append(lines, i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlStatusPendingOne, nil))
	} else if status.PendingApprovals > 1 {
		lines = append(lines, i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlStatusPendingMany, map[string]string{
			"count": fmt.Sprintf("%d", status.PendingApprovals),
		}))
	}

	return strings.Join(lines, "\n")
}

func displayRunID(runID string) string {
	if len(runID) <= 8 {
		return runID
	}
	return runID[:8]
}

func displayObjective(languageHint string, objective string) string {
	trimmed := strings.TrimSpace(objective)
	if trimmed == "" {
		return i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlStatusNoObjective, nil)
	}
	return trimmed
}

func formatResetOutcome(languageHint string, outcome runtime.ConversationResetOutcome) string {
	switch outcome {
	case runtime.ConversationResetMissing:
		return i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlResetMissing, nil)
	case runtime.ConversationResetBusy:
		return i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlResetBusy, nil)
	default:
		return i18n.DefaultCatalog.Format(languageHint, i18n.MessageControlResetCleared, nil)
	}
}

func DefaultCommandSpecs() []CommandSpec {
	specs := make([]CommandSpec, len(defaultCommandSpecs))
	copy(specs, defaultCommandSpecs)
	return specs
}
