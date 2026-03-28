package zalopersonal

import (
	"context"
	"testing"

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
