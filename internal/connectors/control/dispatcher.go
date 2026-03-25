package control

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
)

var defaultRegistry = NewRegistry("start", "help", "status")

// TODO(m3): Add /cancel once inbound runs execute off their transport loops and
// own cancellable contexts that can interrupt an active provider turn.

type ConversationInspector interface {
	InspectConversation(ctx context.Context, key conversations.ConversationKey) (runtime.ConversationStatus, error)
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
		return helpText(), true, nil
	case "status":
		status, err := d.statuses.InspectConversation(ctx, conversations.ConversationKey{
			ConnectorID: env.ConnectorID,
			AccountID:   env.AccountID,
			ExternalID:  env.ConversationID,
			ThreadID:    env.ThreadID,
		})
		if err != nil {
			return "", false, fmt.Errorf("control: inspect conversation status: %w", err)
		}
		return formatConversationStatus(status), true, nil
	default:
		return "", false, nil
	}
}

func helpText() string {
	return strings.Join([]string{
		"Message me naturally to start a task.",
		"",
		"Native commands:",
		"/help   Show this help",
		"/status Show the latest status for this chat",
	}, "\n")
}

func formatConversationStatus(status runtime.ConversationStatus) string {
	if !status.Exists {
		return strings.Join([]string{
			"No runs yet for this chat.",
			"Message me naturally to start one.",
		}, "\n")
	}

	lines := make([]string, 0, 4)
	if status.ActiveRun.ID != "" {
		lines = append(lines, fmt.Sprintf(
			"Active run %s is working on: %s",
			displayRunID(status.ActiveRun.ID),
			displayObjective(status.ActiveRun.Objective),
		))
	} else {
		lines = append(lines, "No active run for this chat.")
	}

	if status.LatestRootRun.ID != "" && status.LatestRootRun.ID != status.ActiveRun.ID {
		lines = append(lines, fmt.Sprintf(
			"Last run %s finished with status %s: %s",
			displayRunID(status.LatestRootRun.ID),
			status.LatestRootRun.Status,
			displayObjective(status.LatestRootRun.Objective),
		))
	}

	if status.PendingApprovals == 1 {
		lines = append(lines, "1 pending approval needs attention in the web UI.")
	} else if status.PendingApprovals > 1 {
		lines = append(lines, fmt.Sprintf("%d pending approvals need attention in the web UI.", status.PendingApprovals))
	}

	return strings.Join(lines, "\n")
}

func displayRunID(runID string) string {
	if len(runID) <= 8 {
		return runID
	}
	return runID[:8]
}

func displayObjective(objective string) string {
	trimmed := strings.TrimSpace(objective)
	if trimmed == "" {
		return "no objective recorded"
	}
	return trimmed
}
