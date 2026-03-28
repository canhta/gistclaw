package zalopersonal

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/connectors/threadstate"
	"github.com/canhta/gistclaw/internal/connectors/zalopersonal/protocol"
	"github.com/canhta/gistclaw/internal/runtime/capabilities"
)

type inboxFlags struct {
	unread   bool
	pinned   bool
	hidden   bool
	archived bool
}

type inboxEntry struct {
	capabilities.InboxEntry
	sortAt time.Time
}

func (c *Connector) CapabilityListInbox(ctx context.Context, req capabilities.InboxListRequest) (capabilities.InboxListResult, error) {
	creds, err := c.loadCapabilityCredentials(ctx)
	if err != nil {
		return capabilities.InboxListResult{}, err
	}

	recentThreads, err := c.loadRecentThreadSummaries(ctx, creds, req.Limit)
	if err != nil {
		return capabilities.InboxListResult{}, err
	}
	friendMap, groupMap, err := c.loadInboxDirectoryMaps(ctx, creds)
	if err != nil {
		return capabilities.InboxListResult{}, err
	}
	flags, err := c.loadInboxFlags(ctx, creds)
	if err != nil {
		return capabilities.InboxListResult{}, err
	}

	entries := make(map[string]*inboxEntry, len(recentThreads)+len(flags))
	for _, summary := range recentThreads {
		entry := inboxEntryFromSummary(summary, friendMap, groupMap, flags[inboxKey(summary.ThreadType, summary.ThreadID)])
		entries[inboxKey(summary.ThreadType, summary.ThreadID)] = &entry
	}

	for key, state := range flags {
		if _, ok := entries[key]; ok {
			continue
		}
		threadType, threadID := splitInboxKey(key)
		entry := inboxEntryFromFlags(threadType, threadID, friendMap, groupMap, state)
		entries[key] = &entry
	}

	query := normalizeCapabilityText(req.Query)
	scope := normalizeInboxScope(req.Scope)
	list := make([]capabilities.InboxEntry, 0, len(entries))
	for _, entry := range entries {
		if req.UnreadOnly && !entry.IsUnread {
			continue
		}
		if !scopeMatchesInbox(scope, entry.ThreadType) {
			continue
		}
		if query != "" && inboxMatchScore(query, entry.InboxEntry) == 0 {
			continue
		}
		list = append(list, entry.InboxEntry)
	}

	sort.Slice(list, func(i, j int) bool {
		left := list[i]
		right := list[j]
		if left.IsUnread != right.IsUnread {
			return left.IsUnread
		}
		if !left.LastMessageAt.Equal(right.LastMessageAt) {
			return left.LastMessageAt.After(right.LastMessageAt)
		}
		if left.Title != right.Title {
			return left.Title < right.Title
		}
		return left.ThreadID < right.ThreadID
	})
	if req.Limit > 0 && len(list) > req.Limit {
		list = list[:req.Limit]
	}

	return capabilities.InboxListResult{
		ConnectorID: c.Metadata().ID,
		Scope:       normalizeInboxScopeResult(scope),
		Entries:     list,
	}, nil
}

func (c *Connector) loadRecentThreadSummaries(ctx context.Context, creds StoredCredentials, limit int) ([]threadstate.Summary, error) {
	if c.threadState == nil {
		return nil, nil
	}
	return c.threadState.List(ctx, threadstate.Filter{
		ConnectorID: c.Metadata().ID,
		AccountID:   creds.AccountID,
		Limit:       limit,
	})
}

func (c *Connector) loadInboxDirectoryMaps(ctx context.Context, creds StoredCredentials) (map[string]protocol.FriendInfo, map[string]protocol.GroupListInfo, error) {
	friendMap := map[string]protocol.FriendInfo{}
	groupMap := map[string]protocol.GroupListInfo{}

	if c.listFriends != nil {
		friends, err := c.listFriends(ctx, creds)
		if err != nil {
			return nil, nil, err
		}
		for _, friend := range friends {
			friendMap[strings.TrimSpace(friend.UserID)] = friend
		}
	}
	if c.listGroups != nil {
		groups, err := c.listGroups(ctx, creds)
		if err != nil {
			return nil, nil, err
		}
		for _, group := range groups {
			groupMap[strings.TrimSpace(group.GroupID)] = group
		}
	}
	return friendMap, groupMap, nil
}

func (c *Connector) loadInboxFlags(ctx context.Context, creds StoredCredentials) (map[string]inboxFlags, error) {
	flags := map[string]inboxFlags{}
	if c.fetchUnreadMarks != nil {
		items, err := c.fetchUnreadMarks(ctx, creds)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			key := inboxKey(threadTypeLabel(item.ThreadType), item.ThreadID)
			current := flags[key]
			current.unread = true
			flags[key] = current
		}
	}
	if c.fetchPinnedThreads != nil {
		items, err := c.fetchPinnedThreads(ctx, creds)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			key := inboxKey(threadTypeLabel(item.ThreadType), item.ThreadID)
			current := flags[key]
			current.pinned = true
			flags[key] = current
		}
	}
	if c.fetchHiddenThreads != nil {
		items, err := c.fetchHiddenThreads(ctx, creds)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			key := inboxKey(threadTypeLabel(item.ThreadType), item.ThreadID)
			current := flags[key]
			current.hidden = true
			flags[key] = current
		}
	}
	if c.fetchArchivedThreads != nil {
		items, err := c.fetchArchivedThreads(ctx, creds)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			key := inboxKey(threadTypeLabel(item.ThreadType), item.ThreadID)
			current := flags[key]
			current.archived = true
			flags[key] = current
		}
	}
	return flags, nil
}

func inboxEntryFromSummary(
	summary threadstate.Summary,
	friends map[string]protocol.FriendInfo,
	groups map[string]protocol.GroupListInfo,
	flags inboxFlags,
) inboxEntry {
	title, subtitle, metadata := resolveInboxIdentity(summary.ThreadType, summary.ThreadID, friends, groups)
	for key, value := range summary.Metadata {
		metadata[key] = value
	}
	applyInboxFlags(metadata, flags)
	return inboxEntry{
		InboxEntry: capabilities.InboxEntry{
			ThreadID:           summary.ThreadID,
			ThreadType:         summary.ThreadType,
			Title:              firstNonEmpty(strings.TrimSpace(summary.Title), title, summary.ThreadID),
			Subtitle:           firstNonEmpty(strings.TrimSpace(summary.Subtitle), subtitle),
			UnreadCount:        boolUnreadCount(flags.unread),
			IsUnread:           flags.unread,
			LastMessagePreview: strings.TrimSpace(summary.LastMessagePreview),
			LastMessageAt:      summary.LastMessageAt.UTC(),
			Metadata:           metadata,
		},
		sortAt: summary.LastMessageAt.UTC(),
	}
}

func inboxEntryFromFlags(
	threadType, threadID string,
	friends map[string]protocol.FriendInfo,
	groups map[string]protocol.GroupListInfo,
	flags inboxFlags,
) inboxEntry {
	title, subtitle, metadata := resolveInboxIdentity(threadType, threadID, friends, groups)
	applyInboxFlags(metadata, flags)
	return inboxEntry{
		InboxEntry: capabilities.InboxEntry{
			ThreadID:    threadID,
			ThreadType:  threadType,
			Title:       firstNonEmpty(title, threadID),
			Subtitle:    subtitle,
			UnreadCount: boolUnreadCount(flags.unread),
			IsUnread:    flags.unread,
			Metadata:    metadata,
		},
	}
}

func resolveInboxIdentity(
	threadType, threadID string,
	friends map[string]protocol.FriendInfo,
	groups map[string]protocol.GroupListInfo,
) (string, string, map[string]string) {
	metadata := map[string]string{}
	switch threadType {
	case "group":
		group, ok := groups[threadID]
		if !ok {
			return threadID, threadID, metadata
		}
		if strings.TrimSpace(group.Avatar) != "" {
			metadata["avatar"] = strings.TrimSpace(group.Avatar)
		}
		return strings.TrimSpace(group.Name), strings.TrimSpace(group.GroupID), metadata
	default:
		friend, ok := friends[threadID]
		if !ok {
			return threadID, threadID, metadata
		}
		if strings.TrimSpace(friend.Avatar) != "" {
			metadata["avatar"] = strings.TrimSpace(friend.Avatar)
		}
		title := strings.TrimSpace(friend.DisplayName)
		if title == "" {
			title = strings.TrimSpace(friend.ZaloName)
		}
		subtitle := strings.TrimSpace(friend.ZaloName)
		if subtitle == "" || strings.EqualFold(subtitle, title) {
			subtitle = strings.TrimSpace(friend.UserID)
		}
		return firstNonEmpty(title, threadID), subtitle, metadata
	}
}

func applyInboxFlags(metadata map[string]string, flags inboxFlags) {
	if flags.pinned {
		metadata["pinned"] = "true"
	}
	if flags.hidden {
		metadata["hidden"] = "true"
	}
	if flags.archived {
		metadata["archived"] = "true"
	}
	if flags.unread {
		metadata["unread_source"] = "mark"
	}
}

func boolUnreadCount(v bool) int {
	if v {
		return 1
	}
	return 0
}

func normalizeInboxScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", "all":
		return "all"
	case "contacts", "direct":
		return "contacts"
	case "groups":
		return "groups"
	default:
		return strings.ToLower(strings.TrimSpace(scope))
	}
}

func normalizeInboxScopeResult(scope string) string {
	if scope == "" {
		return "all"
	}
	return scope
}

func scopeMatchesInbox(scope, threadType string) bool {
	switch scope {
	case "all", "":
		return true
	case "contacts":
		return threadType != "group"
	case "groups":
		return threadType == "group"
	default:
		return false
	}
}

func inboxKey(threadType, threadID string) string {
	return strings.TrimSpace(threadType) + ":" + strings.TrimSpace(threadID)
}

func splitInboxKey(key string) (string, string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "contact", key
	}
	return parts[0], parts[1]
}

func inboxMatchScore(query string, entry capabilities.InboxEntry) float64 {
	if query == "" {
		return 1
	}
	title := normalizeCapabilityText(entry.Title)
	subtitle := normalizeCapabilityText(entry.Subtitle)
	threadID := normalizeCapabilityText(entry.ThreadID)
	switch {
	case title == query:
		return 1
	case strings.Contains(title, query):
		return 0.9
	case subtitle == query:
		return 0.8
	case strings.Contains(subtitle, query):
		return 0.7
	case threadID == query:
		return 0.6
	case strings.Contains(threadID, query):
		return 0.5
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
