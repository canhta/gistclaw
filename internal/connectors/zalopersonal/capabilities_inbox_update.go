package zalopersonal

import (
	"context"
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/connectors/zalopersonal/protocol"
	"github.com/canhta/gistclaw/internal/runtime/capabilities"
)

func (c *Connector) CapabilityUpdateInbox(ctx context.Context, req capabilities.InboxUpdateRequest) (capabilities.InboxUpdateResult, error) {
	creds, err := c.loadCapabilityCredentials(ctx)
	if err != nil {
		return capabilities.InboxUpdateResult{}, err
	}

	threadID := strings.TrimSpace(req.ThreadID)
	if threadID == "" {
		return capabilities.InboxUpdateResult{}, fmt.Errorf("zalo personal capabilities: thread_id is required")
	}
	threadType, err := capabilityThreadType(req.ThreadType)
	if err != nil {
		return capabilities.InboxUpdateResult{}, err
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "mark_read":
		if c.updateUnreadMark == nil {
			return capabilities.InboxUpdateResult{}, fmt.Errorf("zalo personal capabilities: unread update is not configured")
		}
		if err := c.updateUnreadMark(ctx, creds, threadID, threadType, false); err != nil {
			return capabilities.InboxUpdateResult{}, err
		}
	case "mark_unread":
		if c.updateUnreadMark == nil {
			return capabilities.InboxUpdateResult{}, fmt.Errorf("zalo personal capabilities: unread update is not configured")
		}
		if err := c.updateUnreadMark(ctx, creds, threadID, threadType, true); err != nil {
			return capabilities.InboxUpdateResult{}, err
		}
	case "pin":
		if c.updatePinnedThread == nil {
			return capabilities.InboxUpdateResult{}, fmt.Errorf("zalo personal capabilities: pin update is not configured")
		}
		if err := c.updatePinnedThread(ctx, creds, threadID, threadType, true); err != nil {
			return capabilities.InboxUpdateResult{}, err
		}
	case "unpin":
		if c.updatePinnedThread == nil {
			return capabilities.InboxUpdateResult{}, fmt.Errorf("zalo personal capabilities: pin update is not configured")
		}
		if err := c.updatePinnedThread(ctx, creds, threadID, threadType, false); err != nil {
			return capabilities.InboxUpdateResult{}, err
		}
	case "archive":
		if c.updateArchivedThread == nil {
			return capabilities.InboxUpdateResult{}, fmt.Errorf("zalo personal capabilities: archive update is not configured")
		}
		if err := c.updateArchivedThread(ctx, creds, threadID, threadType, true); err != nil {
			return capabilities.InboxUpdateResult{}, err
		}
	case "unarchive":
		if c.updateArchivedThread == nil {
			return capabilities.InboxUpdateResult{}, fmt.Errorf("zalo personal capabilities: archive update is not configured")
		}
		if err := c.updateArchivedThread(ctx, creds, threadID, threadType, false); err != nil {
			return capabilities.InboxUpdateResult{}, err
		}
	case "hide":
		if c.updateHiddenThread == nil {
			return capabilities.InboxUpdateResult{}, fmt.Errorf("zalo personal capabilities: hidden update is not configured")
		}
		if err := c.updateHiddenThread(ctx, creds, threadID, threadType, true); err != nil {
			return capabilities.InboxUpdateResult{}, err
		}
	case "unhide":
		if c.updateHiddenThread == nil {
			return capabilities.InboxUpdateResult{}, fmt.Errorf("zalo personal capabilities: hidden update is not configured")
		}
		if err := c.updateHiddenThread(ctx, creds, threadID, threadType, false); err != nil {
			return capabilities.InboxUpdateResult{}, err
		}
	default:
		return capabilities.InboxUpdateResult{}, fmt.Errorf("zalo personal capabilities: unsupported inbox action %q", strings.TrimSpace(req.Action))
	}

	return capabilities.InboxUpdateResult{
		ConnectorID: c.Metadata().ID,
		ThreadID:    threadID,
		ThreadType:  threadTypeLabel(threadType),
		Action:      action,
		Applied:     true,
		Summary:     "conversation updated",
	}, nil
}

func capabilityThreadType(threadType string) (protocol.ThreadType, error) {
	switch strings.ToLower(strings.TrimSpace(threadType)) {
	case "", "contact", "user", "direct":
		return protocol.ThreadTypeUser, nil
	case "group":
		return protocol.ThreadTypeGroup, nil
	default:
		return 0, fmt.Errorf("zalo personal capabilities: unsupported thread type %q", strings.TrimSpace(threadType))
	}
}
