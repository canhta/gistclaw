package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func TestUpdateUnreadMark(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

	cases := []struct {
		name        string
		unread      bool
		wantPath    string
		wantGroup   bool
		wantDataKey string
	}{
		{name: "mark unread for contact", unread: true, wantPath: "/api/conv/addUnreadMark", wantGroup: false, wantDataKey: "convsUser"},
		{name: "mark read for group", unread: false, wantPath: "/api/conv/removeUnreadMark", wantGroup: true, wantDataKey: "convsGroupData"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldTransport := defaultHTTPTransport
			defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if req.URL.Path != tc.wantPath {
					t.Fatalf("expected %s, got %s", tc.wantPath, req.URL.Path)
				}
				bodyBytes, err := ioReadAll(req.Body)
				if err != nil {
					t.Fatalf("ReadAll body: %v", err)
				}
				values, err := url.ParseQuery(string(bodyBytes))
				if err != nil {
					t.Fatalf("ParseQuery: %v", err)
				}
				params := values.Get("params")
				if params == "" {
					t.Fatal("expected encrypted params form field")
				}

				plain, err := DecodeAESCBC([]byte("0123456789abcdef0123456789abcdef"), params)
				if err != nil {
					t.Fatalf("DecodeAESCBC payload: %v", err)
				}
				var payload map[string]json.RawMessage
				if err := json.Unmarshal(plain, &payload); err != nil {
					t.Fatalf("Unmarshal payload: %v", err)
				}
				var outerText string
				if err := json.Unmarshal(payload["param"], &outerText); err != nil {
					t.Fatalf("Unmarshal outer param text: %v", err)
				}
				var outer map[string]any
				if err := json.Unmarshal([]byte(outerText), &outer); err != nil {
					t.Fatalf("Unmarshal outer param: %v", err)
				}
				if _, ok := outer[tc.wantDataKey]; !ok {
					t.Fatalf("expected payload key %q, got %+v", tc.wantDataKey, outer)
				}
				if tc.unread && outer["imei"] != "imei-123" {
					t.Fatalf("expected imei in unread payload, got %+v", outer)
				}

				return rawHTTPResponse(t, `{"error_code":0,"data":{"data":"{\"updateId\":1}","status":1}}`), nil
			})
			t.Cleanup(func() { defaultHTTPTransport = oldTransport })

			sess := newInboxTestSession(key)
			threadType := ThreadTypeUser
			if tc.wantGroup {
				threadType = ThreadTypeGroup
			}
			if err := UpdateUnreadMark(context.Background(), sess, "user-1", threadType, tc.unread); err != nil {
				t.Fatalf("UpdateUnreadMark: %v", err)
			}
		})
	}
}

func TestUpdateConversationState(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

	cases := []struct {
		name     string
		path     string
		call     func(context.Context, *Session) error
		validate func(*testing.T, map[string]any)
	}{
		{
			name: "pin",
			path: "/api/pinconvers/updatev2",
			call: func(ctx context.Context, sess *Session) error {
				return UpdatePinnedConversation(ctx, sess, "123", ThreadTypeUser, true)
			},
			validate: func(t *testing.T, payload map[string]any) {
				if payload["actionType"] != float64(1) {
					t.Fatalf("expected pin actionType=1, got %+v", payload)
				}
				conversations, ok := payload["conversations"].([]any)
				if !ok || len(conversations) != 1 || conversations[0] != "u123" {
					t.Fatalf("unexpected pin payload: %+v", payload)
				}
			},
		},
		{
			name: "archive",
			path: "/api/archivedchat/update",
			call: func(ctx context.Context, sess *Session) error {
				return UpdateArchivedConversation(ctx, sess, "456", ThreadTypeGroup, true)
			},
			validate: func(t *testing.T, payload map[string]any) {
				if payload["actionType"] != float64(0) || payload["imei"] != "imei-123" {
					t.Fatalf("unexpected archive payload: %+v", payload)
				}
				ids, ok := payload["ids"].([]any)
				if !ok || len(ids) != 1 {
					t.Fatalf("unexpected archive ids: %+v", payload)
				}
				idMap, ok := ids[0].(map[string]any)
				if !ok || idMap["id"] != "456" || idMap["type"] != float64(1) {
					t.Fatalf("unexpected archive id entry: %+v", ids[0])
				}
			},
		},
		{
			name: "hide",
			path: "/api/hiddenconvers/add-remove",
			call: func(ctx context.Context, sess *Session) error {
				return UpdateHiddenConversation(ctx, sess, "789", ThreadTypeGroup, true)
			},
			validate: func(t *testing.T, payload map[string]any) {
				if payload["imei"] != "imei-123" {
					t.Fatalf("expected imei in hidden payload, got %+v", payload)
				}
				addThreads, ok := payload["add_threads"].(string)
				if !ok || addThreads == "" {
					t.Fatalf("expected add_threads payload, got %+v", payload)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldTransport := defaultHTTPTransport
			defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if req.URL.Path != tc.path {
					t.Fatalf("expected %s, got %s", tc.path, req.URL.Path)
				}
				bodyBytes, err := ioReadAll(req.Body)
				if err != nil {
					t.Fatalf("ReadAll body: %v", err)
				}
				values, err := url.ParseQuery(string(bodyBytes))
				if err != nil {
					t.Fatalf("ParseQuery: %v", err)
				}
				params := values.Get("params")
				if params == "" {
					t.Fatal("expected encrypted params form field")
				}

				plain, err := DecodeAESCBC([]byte("0123456789abcdef0123456789abcdef"), params)
				if err != nil {
					t.Fatalf("DecodeAESCBC payload: %v", err)
				}
				var payload map[string]any
				if err := json.Unmarshal(plain, &payload); err != nil {
					t.Fatalf("Unmarshal payload: %v", err)
				}
				tc.validate(t, payload)

				return rawHTTPResponse(t, `{"error_code":0,"data":""}`), nil
			})
			t.Cleanup(func() { defaultHTTPTransport = oldTransport })

			if err := tc.call(context.Background(), newInboxTestSession(key)); err != nil {
				t.Fatalf("state update call: %v", err)
			}
		})
	}
}
