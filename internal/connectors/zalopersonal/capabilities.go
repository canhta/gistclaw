package zalopersonal

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/canhta/gistclaw/internal/runtime/capabilities"
)

func (c *Connector) CapabilityListDirectory(ctx context.Context, req capabilities.DirectoryListRequest) (capabilities.DirectoryListResult, error) {
	creds, err := c.loadCapabilityCredentials(ctx)
	if err != nil {
		return capabilities.DirectoryListResult{}, err
	}
	entries, scope, err := c.listDirectoryEntries(ctx, creds, req.Scope)
	if err != nil {
		return capabilities.DirectoryListResult{}, err
	}

	query := normalizeCapabilityText(req.Query)
	if query != "" {
		filtered := make([]capabilities.DirectoryEntry, 0, len(entries))
		for _, entry := range entries {
			if capabilityMatchScore(query, entry) == 0 {
				continue
			}
			filtered = append(filtered, entry)
		}
		entries = filtered
	}

	sortDirectoryEntries(entries)
	if req.Limit > 0 && len(entries) > req.Limit {
		entries = entries[:req.Limit]
	}
	return capabilities.DirectoryListResult{
		ConnectorID: c.Metadata().ID,
		Scope:       scope,
		Entries:     entries,
	}, nil
}

func (c *Connector) CapabilityResolveTarget(ctx context.Context, req capabilities.TargetResolveRequest) (capabilities.TargetResolveResult, error) {
	creds, err := c.loadCapabilityCredentials(ctx)
	if err != nil {
		return capabilities.TargetResolveResult{}, err
	}
	query := normalizeCapabilityText(req.Query)
	if query == "" {
		return capabilities.TargetResolveResult{}, fmt.Errorf("zalo personal capabilities: query is required")
	}

	entries, _, err := c.listDirectoryEntries(ctx, creds, req.Scope)
	if err != nil {
		return capabilities.TargetResolveResult{}, err
	}

	matches := make([]capabilities.TargetMatch, 0, len(entries))
	for _, entry := range entries {
		score := capabilityMatchScore(query, entry)
		if score == 0 {
			continue
		}
		matches = append(matches, capabilities.TargetMatch{
			ID:       entry.ID,
			Title:    entry.Title,
			Subtitle: entry.Subtitle,
			Kind:     entry.Kind,
			Score:    score,
			Metadata: cloneStringMap(entry.Metadata),
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		if matches[i].Title != matches[j].Title {
			return matches[i].Title < matches[j].Title
		}
		return matches[i].ID < matches[j].ID
	})
	if req.Limit > 0 && len(matches) > req.Limit {
		matches = matches[:req.Limit]
	}
	return capabilities.TargetResolveResult{
		ConnectorID: c.Metadata().ID,
		Query:       strings.TrimSpace(req.Query),
		Matches:     matches,
	}, nil
}

func (c *Connector) CapabilitySend(ctx context.Context, req capabilities.SendRequest) (capabilities.SendResult, error) {
	targetID := strings.TrimSpace(req.TargetID)
	targetType := strings.TrimSpace(req.TargetType)
	if targetID == "" {
		return capabilities.SendResult{}, fmt.Errorf("zalo personal capabilities: target_id is required")
	}
	if strings.TrimSpace(req.Message) == "" {
		return capabilities.SendResult{}, fmt.Errorf("zalo personal capabilities: message is required")
	}
	if targetType == "group" && !c.groupPolicy.Allowlist[targetID] {
		return capabilities.SendResult{}, fmt.Errorf("zalo personal capabilities: group target is not enabled for direct send")
	}
	if _, err := c.loadCapabilityCredentials(ctx); err != nil {
		return capabilities.SendResult{}, err
	}
	if err := c.SendText(ctx, targetID, req.Message); err != nil {
		return capabilities.SendResult{}, err
	}
	return capabilities.SendResult{
		ConnectorID: c.Metadata().ID,
		TargetID:    targetID,
		TargetType:  targetType,
		Accepted:    true,
		Summary:     "message sent",
	}, nil
}

func (c *Connector) loadCapabilityCredentials(ctx context.Context) (StoredCredentials, error) {
	creds, ok, err := LoadStoredCredentials(ctx, c.outbound.db)
	if err != nil {
		return StoredCredentials{}, err
	}
	if !ok {
		return StoredCredentials{}, fmt.Errorf("zalo personal capabilities: not authenticated")
	}
	return creds, nil
}

func (c *Connector) listDirectoryEntries(ctx context.Context, creds StoredCredentials, scope string) ([]capabilities.DirectoryEntry, string, error) {
	switch normalizeCapabilityScope(scope) {
	case "", "all":
		friendEntries, err := c.friendDirectoryEntries(ctx, creds)
		if err != nil {
			return nil, "", err
		}
		groupEntries, err := c.groupDirectoryEntries(ctx, creds)
		if err != nil {
			return nil, "", err
		}
		return append(friendEntries, groupEntries...), "all", nil
	case "contacts":
		entries, err := c.friendDirectoryEntries(ctx, creds)
		if err != nil {
			return nil, "", err
		}
		return entries, "contacts", nil
	case "groups":
		entries, err := c.groupDirectoryEntries(ctx, creds)
		if err != nil {
			return nil, "", err
		}
		return entries, "groups", nil
	default:
		return nil, "", fmt.Errorf("zalo personal capabilities: unsupported scope %q", strings.TrimSpace(scope))
	}
}

func (c *Connector) friendDirectoryEntries(ctx context.Context, creds StoredCredentials) ([]capabilities.DirectoryEntry, error) {
	if c.listFriends == nil {
		return nil, fmt.Errorf("zalo personal capabilities: contacts are not configured")
	}
	friends, err := c.listFriends(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("zalo personal capabilities: list contacts: %w", err)
	}
	entries := make([]capabilities.DirectoryEntry, 0, len(friends))
	for _, friend := range friends {
		title := strings.TrimSpace(friend.DisplayName)
		if title == "" {
			title = strings.TrimSpace(friend.ZaloName)
		}
		if title == "" {
			title = strings.TrimSpace(friend.UserID)
		}
		subtitle := strings.TrimSpace(friend.ZaloName)
		if subtitle == "" || strings.EqualFold(subtitle, title) {
			subtitle = strings.TrimSpace(friend.UserID)
		}
		metadata := map[string]string{}
		if avatar := strings.TrimSpace(friend.Avatar); avatar != "" {
			metadata["avatar"] = avatar
		}
		entries = append(entries, capabilities.DirectoryEntry{
			ID:       strings.TrimSpace(friend.UserID),
			Title:    title,
			Subtitle: subtitle,
			Kind:     "contact",
			Metadata: metadata,
		})
	}
	return entries, nil
}

func (c *Connector) groupDirectoryEntries(ctx context.Context, creds StoredCredentials) ([]capabilities.DirectoryEntry, error) {
	if c.listGroups == nil {
		return nil, fmt.Errorf("zalo personal capabilities: groups are not configured")
	}
	groups, err := c.listGroups(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("zalo personal capabilities: list groups: %w", err)
	}
	entries := make([]capabilities.DirectoryEntry, 0, len(groups))
	for _, group := range groups {
		metadata := map[string]string{}
		if avatar := strings.TrimSpace(group.Avatar); avatar != "" {
			metadata["avatar"] = avatar
		}
		if group.TotalMember > 0 {
			metadata["total_member"] = fmt.Sprintf("%d", group.TotalMember)
		}
		entries = append(entries, capabilities.DirectoryEntry{
			ID:       strings.TrimSpace(group.GroupID),
			Title:    strings.TrimSpace(group.Name),
			Subtitle: strings.TrimSpace(group.GroupID),
			Kind:     "group",
			Metadata: metadata,
		})
	}
	return entries, nil
}

func normalizeCapabilityScope(scope string) string {
	return strings.ToLower(strings.TrimSpace(scope))
}

func normalizeCapabilityText(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}

func capabilityMatchScore(query string, entry capabilities.DirectoryEntry) float64 {
	if query == "" {
		return 1
	}
	title := normalizeCapabilityText(entry.Title)
	subtitle := normalizeCapabilityText(entry.Subtitle)
	entryID := normalizeCapabilityText(entry.ID)

	switch {
	case query == title || query == entryID:
		return 1
	case subtitle != "" && query == subtitle:
		return 0.95
	case strings.HasPrefix(title, query):
		return 0.9
	case subtitle != "" && strings.HasPrefix(subtitle, query):
		return 0.85
	case strings.Contains(title, query):
		return 0.8
	case strings.Contains(subtitle, query) || strings.Contains(entryID, query):
		return 0.7
	default:
		return 0
	}
}

func sortDirectoryEntries(entries []capabilities.DirectoryEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Title != entries[j].Title {
			return entries[i].Title < entries[j].Title
		}
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].ID < entries[j].ID
	})
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

var _ capabilities.DirectoryAdapter = (*Connector)(nil)
var _ capabilities.TargetResolver = (*Connector)(nil)
var _ capabilities.Sender = (*Connector)(nil)
