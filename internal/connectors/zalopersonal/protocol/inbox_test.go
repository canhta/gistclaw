package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func TestFetchUnreadMarks(t *testing.T) {
	keyBytes := []byte("0123456789abcdef0123456789abcdef")
	key := base64.StdEncoding.EncodeToString(keyBytes)

	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", req.Method)
		}
		if req.URL.Path != "/api/conv/getUnreadMark" {
			t.Fatalf("expected unread mark path, got %s", req.URL.Path)
		}

		inner, err := json.Marshal(Response[json.RawMessage]{
			Data: json.RawMessage(`{
				"convsUser":[{"id":123,"ts":1700000000000}],
				"convsGroup":[{"id":456,"ts":1700000060000}]
			}`),
		})
		if err != nil {
			t.Fatalf("Marshal inner response: %v", err)
		}
		encrypted, err := EncodeAESCBC(keyBytes, string(inner), false)
		if err != nil {
			t.Fatalf("EncodeAESCBC response: %v", err)
		}
		escaped := url.PathEscape(encrypted)
		return jsonHTTPResponse(t, Response[*string]{Data: &escaped}), nil
	})
	t.Cleanup(func() { defaultHTTPTransport = oldTransport })

	sess := newInboxTestSession(key)
	items, err := FetchUnreadMarks(context.Background(), sess)
	if err != nil {
		t.Fatalf("FetchUnreadMarks: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 unread items, got %+v", items)
	}
	if items[0].ThreadID != "123" || items[0].ThreadType != ThreadTypeUser {
		t.Fatalf("unexpected first unread item: %+v", items[0])
	}
	if items[1].ThreadID != "456" || items[1].ThreadType != ThreadTypeGroup {
		t.Fatalf("unexpected second unread item: %+v", items[1])
	}
}

func TestFetchPinnedConversations(t *testing.T) {
	keyBytes := []byte("0123456789abcdef0123456789abcdef")
	key := base64.StdEncoding.EncodeToString(keyBytes)

	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/pinconvers/list" {
			t.Fatalf("expected pin list path, got %s", req.URL.Path)
		}
		if req.URL.Query().Get("params") == "" {
			t.Fatal("expected encrypted params")
		}

		inner, err := json.Marshal(Response[json.RawMessage]{
			Data: json.RawMessage(`{"conversations":["u123","g456"]}`),
		})
		if err != nil {
			t.Fatalf("Marshal inner response: %v", err)
		}
		encrypted, err := EncodeAESCBC(keyBytes, string(inner), false)
		if err != nil {
			t.Fatalf("EncodeAESCBC response: %v", err)
		}
		escaped := url.PathEscape(encrypted)
		return jsonHTTPResponse(t, Response[*string]{Data: &escaped}), nil
	})
	t.Cleanup(func() { defaultHTTPTransport = oldTransport })

	sess := newInboxTestSession(key)
	items, err := FetchPinnedConversations(context.Background(), sess)
	if err != nil {
		t.Fatalf("FetchPinnedConversations: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 pinned items, got %+v", items)
	}
	if items[0].ThreadID != "123" || items[0].ThreadType != ThreadTypeUser {
		t.Fatalf("unexpected first pinned item: %+v", items[0])
	}
	if items[1].ThreadID != "456" || items[1].ThreadType != ThreadTypeGroup {
		t.Fatalf("unexpected second pinned item: %+v", items[1])
	}
}

func TestFetchHiddenAndArchivedConversations(t *testing.T) {
	keyBytes := []byte("0123456789abcdef0123456789abcdef")
	key := base64.StdEncoding.EncodeToString(keyBytes)

	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/hiddenconvers/get-all":
			inner, err := json.Marshal(Response[json.RawMessage]{
				Data: json.RawMessage(`{
					"threads":[
						{"thread_id":"123","is_group":0},
						{"thread_id":"456","is_group":1}
					]
				}`),
			})
			if err != nil {
				t.Fatalf("Marshal hidden response: %v", err)
			}
			encrypted, err := EncodeAESCBC(keyBytes, string(inner), false)
			if err != nil {
				t.Fatalf("EncodeAESCBC hidden response: %v", err)
			}
			escaped := url.PathEscape(encrypted)
			return jsonHTTPResponse(t, Response[*string]{Data: &escaped}), nil
		case "/api/archivedchat/list":
			inner, err := json.Marshal(Response[json.RawMessage]{
				Data: json.RawMessage(`{
					"items":[
						"u789",
						{"thread_id":"456","is_group":1},
						{"conversationId":"123","isGroup":"false"}
					]
				}`),
			})
			if err != nil {
				t.Fatalf("Marshal archived response: %v", err)
			}
			encrypted, err := EncodeAESCBC(keyBytes, string(inner), false)
			if err != nil {
				t.Fatalf("EncodeAESCBC archived response: %v", err)
			}
			escaped := url.PathEscape(encrypted)
			return jsonHTTPResponse(t, Response[*string]{Data: &escaped}), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	})
	t.Cleanup(func() { defaultHTTPTransport = oldTransport })

	sess := newInboxTestSession(key)
	hidden, err := FetchHiddenConversations(context.Background(), sess)
	if err != nil {
		t.Fatalf("FetchHiddenConversations: %v", err)
	}
	if len(hidden) != 2 || hidden[0].ThreadID != "123" || hidden[1].ThreadType != ThreadTypeGroup {
		t.Fatalf("unexpected hidden conversations: %+v", hidden)
	}

	archived, err := FetchArchivedConversations(context.Background(), sess)
	if err != nil {
		t.Fatalf("FetchArchivedConversations: %v", err)
	}
	if len(archived) != 3 {
		t.Fatalf("expected 3 archived items, got %+v", archived)
	}
	if archived[0].ThreadID != "789" || archived[0].ThreadType != ThreadTypeUser {
		t.Fatalf("unexpected first archived item: %+v", archived[0])
	}
	if archived[1].ThreadID != "456" || archived[1].ThreadType != ThreadTypeGroup {
		t.Fatalf("unexpected second archived item: %+v", archived[1])
	}
}

func TestUnmarshalProtocolDataHandlesQuotedJSON(t *testing.T) {
	var payload struct {
		Value string `json:"value"`
	}
	if err := unmarshalProtocolData([]byte(`"{\"value\":\"ok\"}"`), &payload); err != nil {
		t.Fatalf("unmarshalProtocolData: %v", err)
	}
	if payload.Value != "ok" {
		t.Fatalf("expected quoted JSON to decode, got %+v", payload)
	}
}

func TestDecodeConversationThreadRejectsUnknownPrefix(t *testing.T) {
	threadID, threadType, ok := decodeConversationThread("x123")
	if ok || threadID != "" || threadType != 0 {
		t.Fatalf("expected unknown prefix to fail, got id=%q type=%v ok=%v", threadID, threadType, ok)
	}
}

func newInboxTestSession(key string) *Session {
	sess := NewSession()
	sess.IMEI = "imei-123"
	sess.UserAgent = DefaultUserAgent
	sess.SecretKey = key
	sess.LoginInfo = &LoginInfo{
		ZpwServiceMapV3: ZpwServiceMapV3{
			Conversation: []string{"https://conversation-wpa.chat.zalo.me"},
			Label:        []string{"https://label-wpa.chat.zalo.me"},
		},
	}
	return sess
}
