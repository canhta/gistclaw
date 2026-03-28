package zalopersonal

import (
	"context"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/connectors/threadstate"
	"github.com/canhta/gistclaw/internal/connectors/zalopersonal/protocol"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/runtime/capabilities"
)

func TestCapabilities_ListDirectory(t *testing.T) {
	db := setupZaloOutboundDB(t)
	if err := SaveStoredCredentials(context.Background(), db, StoredCredentials{
		AccountID:   "acc-1",
		DisplayName: "Canh",
		IMEI:        "imei-1",
		Cookie:      "cookie-1",
		UserAgent:   "ua-1",
	}); err != nil {
		t.Fatalf("SaveStoredCredentials: %v", err)
	}

	connector := NewConnector(db, conversations.NewConversationStore(db), &stubInboundRuntime{}, "assistant")
	connector.listFriends = func(_ context.Context, creds StoredCredentials) ([]protocol.FriendInfo, error) {
		if creds.AccountID != "acc-1" {
			t.Fatalf("unexpected creds: %+v", creds)
		}
		return []protocol.FriendInfo{
			{UserID: "user-2", DisplayName: "Bao", ZaloName: "Bao Nguyen"},
			{UserID: "user-1", DisplayName: "Anh An"},
		}, nil
	}
	connector.listGroups = func(context.Context, StoredCredentials) ([]protocol.GroupListInfo, error) {
		return []protocol.GroupListInfo{
			{GroupID: "group-1", Name: "Alpha Team", TotalMember: 3},
		}, nil
	}

	result, err := connector.CapabilityListDirectory(context.Background(), capabilities.DirectoryListRequest{
		ConnectorID: "zalo_personal",
		Scope:       "all",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("CapabilityListDirectory: %v", err)
	}
	if len(result.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %+v", result.Entries)
	}
	if result.Entries[0].Title != "Alpha Team" || result.Entries[0].Kind != "group" {
		t.Fatalf("expected alphabetically sorted entries, got %+v", result.Entries)
	}
	if result.Entries[1].ID != "user-1" || result.Entries[1].Kind != "contact" {
		t.Fatalf("unexpected contact entry: %+v", result.Entries[1])
	}
}

func TestCapabilities_ListInbox(t *testing.T) {
	db := setupZaloOutboundDB(t)
	if err := SaveStoredCredentials(context.Background(), db, StoredCredentials{
		AccountID:   "acc-1",
		DisplayName: "Canh",
		IMEI:        "imei-1",
		Cookie:      "cookie-1",
		UserAgent:   "ua-1",
	}); err != nil {
		t.Fatalf("SaveStoredCredentials: %v", err)
	}

	connector := NewConnector(db, conversations.NewConversationStore(db), &stubInboundRuntime{}, "assistant")
	now := time.Unix(1700000000, 0).UTC()
	if err := connector.threadState.Upsert(context.Background(), threadstate.Summary{
		ConnectorID:        "zalo_personal",
		AccountID:          "acc-1",
		ThreadID:           "user-1",
		ThreadType:         "contact",
		LastMessagePreview: "alo",
		LastMessageAt:      now,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	connector.listFriends = func(context.Context, StoredCredentials) ([]protocol.FriendInfo, error) {
		return []protocol.FriendInfo{
			{UserID: "user-1", DisplayName: "Mẹ"},
		}, nil
	}
	connector.listGroups = func(context.Context, StoredCredentials) ([]protocol.GroupListInfo, error) {
		return []protocol.GroupListInfo{
			{GroupID: "group-1", Name: "Gia đình"},
		}, nil
	}
	connector.fetchPinnedThreads = func(context.Context, StoredCredentials) ([]protocol.PinnedConversationInfo, error) {
		return nil, nil
	}
	connector.fetchHiddenThreads = func(context.Context, StoredCredentials) ([]protocol.HiddenConversationInfo, error) {
		return nil, nil
	}
	connector.fetchArchivedThreads = func(context.Context, StoredCredentials) ([]protocol.ArchivedConversationInfo, error) {
		return nil, nil
	}
	connector.fetchUnreadMarks = func(context.Context, StoredCredentials) ([]protocol.UnreadMarkInfo, error) {
		return []protocol.UnreadMarkInfo{
			{ThreadID: "user-1", ThreadType: protocol.ThreadTypeUser, MarkedAt: now.Add(2 * time.Minute)},
			{ThreadID: "group-1", ThreadType: protocol.ThreadTypeGroup, MarkedAt: now.Add(time.Minute)},
		}, nil
	}

	result, err := connector.CapabilityListInbox(context.Background(), capabilities.InboxListRequest{
		ConnectorID: "zalo_personal",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("CapabilityListInbox: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 inbox entries, got %+v", result.Entries)
	}
	if result.Entries[0].ThreadID != "user-1" || !result.Entries[0].IsUnread || result.Entries[0].Title != "Mẹ" {
		t.Fatalf("unexpected first inbox entry: %+v", result.Entries[0])
	}
	if result.Entries[1].ThreadID != "group-1" || result.Entries[1].Title != "Gia đình" || !result.Entries[1].IsUnread {
		t.Fatalf("unexpected second inbox entry: %+v", result.Entries[1])
	}
}

func TestCapabilities_ListInboxUnreadOnly(t *testing.T) {
	db := setupZaloOutboundDB(t)
	if err := SaveStoredCredentials(context.Background(), db, StoredCredentials{
		AccountID:   "acc-1",
		DisplayName: "Canh",
		IMEI:        "imei-1",
		Cookie:      "cookie-1",
		UserAgent:   "ua-1",
	}); err != nil {
		t.Fatalf("SaveStoredCredentials: %v", err)
	}

	connector := NewConnector(db, conversations.NewConversationStore(db), &stubInboundRuntime{}, "assistant")
	now := time.Unix(1700000000, 0).UTC()
	if err := connector.threadState.Upsert(context.Background(), threadstate.Summary{
		ConnectorID:        "zalo_personal",
		AccountID:          "acc-1",
		ThreadID:           "user-1",
		ThreadType:         "contact",
		LastMessagePreview: "alo",
		LastMessageAt:      now,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	connector.listFriends = func(context.Context, StoredCredentials) ([]protocol.FriendInfo, error) {
		return []protocol.FriendInfo{
			{UserID: "user-1", DisplayName: "Mẹ"},
		}, nil
	}
	connector.listGroups = func(context.Context, StoredCredentials) ([]protocol.GroupListInfo, error) {
		return nil, nil
	}
	connector.fetchUnreadMarks = func(context.Context, StoredCredentials) ([]protocol.UnreadMarkInfo, error) {
		return nil, nil
	}
	connector.fetchPinnedThreads = func(context.Context, StoredCredentials) ([]protocol.PinnedConversationInfo, error) {
		return nil, nil
	}
	connector.fetchHiddenThreads = func(context.Context, StoredCredentials) ([]protocol.HiddenConversationInfo, error) {
		return nil, nil
	}
	connector.fetchArchivedThreads = func(context.Context, StoredCredentials) ([]protocol.ArchivedConversationInfo, error) {
		return nil, nil
	}

	result, err := connector.CapabilityListInbox(context.Background(), capabilities.InboxListRequest{
		ConnectorID: "zalo_personal",
		UnreadOnly:  true,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("CapabilityListInbox: %v", err)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("expected unread-only inbox to filter entries, got %+v", result.Entries)
	}
}

func TestCapabilities_ListInboxIncludesThreadFlags(t *testing.T) {
	db := setupZaloOutboundDB(t)
	if err := SaveStoredCredentials(context.Background(), db, StoredCredentials{
		AccountID:   "acc-1",
		DisplayName: "Canh",
		IMEI:        "imei-1",
		Cookie:      "cookie-1",
		UserAgent:   "ua-1",
	}); err != nil {
		t.Fatalf("SaveStoredCredentials: %v", err)
	}

	connector := NewConnector(db, conversations.NewConversationStore(db), &stubInboundRuntime{}, "assistant")
	connector.listFriends = func(context.Context, StoredCredentials) ([]protocol.FriendInfo, error) {
		return []protocol.FriendInfo{
			{UserID: "user-1", DisplayName: "Mẹ", Avatar: "https://example.com/me.png"},
		}, nil
	}
	connector.listGroups = func(context.Context, StoredCredentials) ([]protocol.GroupListInfo, error) {
		return nil, nil
	}
	connector.fetchUnreadMarks = func(context.Context, StoredCredentials) ([]protocol.UnreadMarkInfo, error) {
		return []protocol.UnreadMarkInfo{
			{ThreadID: "user-1", ThreadType: protocol.ThreadTypeUser},
		}, nil
	}
	connector.fetchPinnedThreads = func(context.Context, StoredCredentials) ([]protocol.PinnedConversationInfo, error) {
		return []protocol.PinnedConversationInfo{
			{ThreadID: "user-1", ThreadType: protocol.ThreadTypeUser},
		}, nil
	}
	connector.fetchHiddenThreads = func(context.Context, StoredCredentials) ([]protocol.HiddenConversationInfo, error) {
		return []protocol.HiddenConversationInfo{
			{ThreadID: "user-1", ThreadType: protocol.ThreadTypeUser},
		}, nil
	}
	connector.fetchArchivedThreads = func(context.Context, StoredCredentials) ([]protocol.ArchivedConversationInfo, error) {
		return []protocol.ArchivedConversationInfo{
			{ThreadID: "user-1", ThreadType: protocol.ThreadTypeUser},
		}, nil
	}

	result, err := connector.CapabilityListInbox(context.Background(), capabilities.InboxListRequest{
		ConnectorID: "zalo_personal",
		UnreadOnly:  true,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("CapabilityListInbox: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 flagged inbox entry, got %+v", result.Entries)
	}
	entry := result.Entries[0]
	if entry.Metadata["pinned"] != "true" || entry.Metadata["hidden"] != "true" || entry.Metadata["archived"] != "true" {
		t.Fatalf("expected thread flags in metadata, got %+v", entry.Metadata)
	}
	if entry.Metadata["avatar"] != "https://example.com/me.png" {
		t.Fatalf("expected directory metadata to be preserved, got %+v", entry.Metadata)
	}
}

func TestCapabilities_UpdateInbox(t *testing.T) {
	db := setupZaloOutboundDB(t)
	if err := SaveStoredCredentials(context.Background(), db, StoredCredentials{
		AccountID:   "acc-1",
		DisplayName: "Canh",
		IMEI:        "imei-1",
		Cookie:      "cookie-1",
		UserAgent:   "ua-1",
	}); err != nil {
		t.Fatalf("SaveStoredCredentials: %v", err)
	}

	connector := NewConnector(db, conversations.NewConversationStore(db), &stubInboundRuntime{}, "assistant")
	var got []string
	connector.updateUnreadMark = func(_ context.Context, creds StoredCredentials, threadID string, threadType protocol.ThreadType, unread bool) error {
		got = append(got, "unread:"+creds.AccountID+":"+threadID+":"+threadTypeLabel(threadType)+":"+boolString(unread))
		return nil
	}
	connector.updatePinnedThread = func(_ context.Context, creds StoredCredentials, threadID string, threadType protocol.ThreadType, enabled bool) error {
		got = append(got, "pin:"+creds.AccountID+":"+threadID+":"+threadTypeLabel(threadType)+":"+boolString(enabled))
		return nil
	}
	connector.updateArchivedThread = func(_ context.Context, creds StoredCredentials, threadID string, threadType protocol.ThreadType, enabled bool) error {
		got = append(got, "archive:"+creds.AccountID+":"+threadID+":"+threadTypeLabel(threadType)+":"+boolString(enabled))
		return nil
	}
	connector.updateHiddenThread = func(_ context.Context, creds StoredCredentials, threadID string, threadType protocol.ThreadType, enabled bool) error {
		got = append(got, "hidden:"+creds.AccountID+":"+threadID+":"+threadTypeLabel(threadType)+":"+boolString(enabled))
		return nil
	}

	cases := []struct {
		name   string
		action string
		want   string
	}{
		{name: "mark read", action: "mark_read", want: "unread:acc-1:user-1:contact:false"},
		{name: "mark unread", action: "mark_unread", want: "unread:acc-1:user-1:contact:true"},
		{name: "pin", action: "pin", want: "pin:acc-1:user-1:contact:true"},
		{name: "unpin", action: "unpin", want: "pin:acc-1:user-1:contact:false"},
		{name: "archive", action: "archive", want: "archive:acc-1:user-1:contact:true"},
		{name: "unarchive", action: "unarchive", want: "archive:acc-1:user-1:contact:false"},
		{name: "hide", action: "hide", want: "hidden:acc-1:user-1:contact:true"},
		{name: "unhide", action: "unhide", want: "hidden:acc-1:user-1:contact:false"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = nil
			result, err := connector.CapabilityUpdateInbox(context.Background(), capabilities.InboxUpdateRequest{
				ConnectorID: "zalo_personal",
				ThreadID:    "user-1",
				ThreadType:  "contact",
				Action:      tc.action,
			})
			if err != nil {
				t.Fatalf("CapabilityUpdateInbox: %v", err)
			}
			if !result.Applied || result.Action != tc.action {
				t.Fatalf("unexpected result: %+v", result)
			}
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("unexpected call trace: %+v", got)
			}
		})
	}
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func TestCapabilities_ResolveTarget(t *testing.T) {
	db := setupZaloOutboundDB(t)
	if err := SaveStoredCredentials(context.Background(), db, StoredCredentials{
		AccountID:   "acc-1",
		DisplayName: "Canh",
		IMEI:        "imei-1",
		Cookie:      "cookie-1",
		UserAgent:   "ua-1",
	}); err != nil {
		t.Fatalf("SaveStoredCredentials: %v", err)
	}

	connector := NewConnector(db, conversations.NewConversationStore(db), &stubInboundRuntime{}, "assistant")
	connector.listFriends = func(context.Context, StoredCredentials) ([]protocol.FriendInfo, error) {
		return []protocol.FriendInfo{
			{UserID: "user-2", DisplayName: "Anh Bao"},
			{UserID: "user-1", DisplayName: "Anh An"},
		}, nil
	}
	connector.listGroups = func(context.Context, StoredCredentials) ([]protocol.GroupListInfo, error) {
		return []protocol.GroupListInfo{
			{GroupID: "group-1", Name: "Anh Em"},
		}, nil
	}

	result, err := connector.CapabilityResolveTarget(context.Background(), capabilities.TargetResolveRequest{
		ConnectorID: "zalo_personal",
		Query:       "Anh An",
		Scope:       "all",
	})
	if err != nil {
		t.Fatalf("CapabilityResolveTarget: %v", err)
	}
	if len(result.Matches) == 0 {
		t.Fatalf("expected matches, got %+v", result)
	}
	if result.Matches[0].ID != "user-1" || result.Matches[0].Kind != "contact" || result.Matches[0].Score != 1 {
		t.Fatalf("expected exact contact match first, got %+v", result.Matches)
	}
}

func TestCapabilities_Send(t *testing.T) {
	db := setupZaloOutboundDB(t)
	if err := SaveStoredCredentials(context.Background(), db, StoredCredentials{
		AccountID:   "acc-1",
		DisplayName: "Canh",
		IMEI:        "imei-1",
		Cookie:      "cookie-1",
		UserAgent:   "ua-1",
	}); err != nil {
		t.Fatalf("SaveStoredCredentials: %v", err)
	}

	connector := NewConnector(db, conversations.NewConversationStore(db), &stubInboundRuntime{}, "assistant")
	var gotChatID string
	var gotText string
	connector.sendText = func(_ context.Context, creds StoredCredentials, chatID, text string) error {
		if creds.AccountID != "acc-1" {
			t.Fatalf("unexpected creds: %+v", creds)
		}
		gotChatID = chatID
		gotText = text
		return nil
	}

	result, err := connector.CapabilitySend(context.Background(), capabilities.SendRequest{
		ConnectorID: "zalo_personal",
		TargetID:    "user-1",
		TargetType:  "contact",
		Message:     "hello from telegram",
	})
	if err != nil {
		t.Fatalf("CapabilitySend: %v", err)
	}
	if !result.Accepted || gotChatID != "user-1" || gotText != "hello from telegram" {
		t.Fatalf("unexpected send result=%+v chat=%q text=%q", result, gotChatID, gotText)
	}
}

func TestCapabilities_SendRejectsGroupWithoutAllowlist(t *testing.T) {
	db := setupZaloOutboundDB(t)
	if err := SaveStoredCredentials(context.Background(), db, StoredCredentials{
		AccountID:   "acc-1",
		DisplayName: "Canh",
		IMEI:        "imei-1",
		Cookie:      "cookie-1",
		UserAgent:   "ua-1",
	}); err != nil {
		t.Fatalf("SaveStoredCredentials: %v", err)
	}

	connector := NewConnector(db, conversations.NewConversationStore(db), &stubInboundRuntime{}, "assistant")
	_, err := connector.CapabilitySend(context.Background(), capabilities.SendRequest{
		ConnectorID: "zalo_personal",
		TargetID:    "group-1",
		TargetType:  "group",
		Message:     "hello group",
	})
	if err == nil || err.Error() != "zalo personal capabilities: group target is not enabled for direct send" {
		t.Fatalf("expected group send policy error, got %v", err)
	}
}

func TestCapabilities_RequireAuthentication(t *testing.T) {
	db := setupZaloOutboundDB(t)
	connector := NewConnector(db, conversations.NewConversationStore(db), &stubInboundRuntime{}, "assistant")

	_, err := connector.CapabilityListDirectory(context.Background(), capabilities.DirectoryListRequest{
		ConnectorID: "zalo_personal",
		Scope:       "contacts",
	})
	if err == nil || err.Error() != "zalo personal capabilities: not authenticated" {
		t.Fatalf("expected not authenticated error, got %v", err)
	}
}
