package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func TestFetchFriends(t *testing.T) {
	keyBytes := []byte("0123456789abcdef0123456789abcdef")
	key := base64.StdEncoding.EncodeToString(keyBytes)

	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", req.Method)
		}
		if req.URL.Path != "/api/social/friend/getfriends" {
			t.Fatalf("expected getfriends path, got %s", req.URL.Path)
		}

		plain, err := DecodeAESCBC(keyBytes, req.URL.Query().Get("params"))
		if err != nil {
			t.Fatalf("DecodeAESCBC payload: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(plain, &payload); err != nil {
			t.Fatalf("Unmarshal payload: %v", err)
		}
		if payload["imei"] != "imei-123" {
			t.Fatalf("expected imei in payload, got %+v", payload)
		}
		if payload["count"] != float64(20000) {
			t.Fatalf("expected count 20000, got %+v", payload)
		}

		inner, err := json.Marshal(Response[json.RawMessage]{
			Data: json.RawMessage(`[
				{"userId":"user-2","displayName":"Bao","zaloName":"Bao Nguyen"},
				{"userId":"user-1","displayName":"An"}
			]`),
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

	sess := NewSession()
	sess.IMEI = "imei-123"
	sess.UserAgent = DefaultUserAgent
	sess.SecretKey = key
	sess.LoginInfo = &LoginInfo{
		ZpwServiceMapV3: ZpwServiceMapV3{
			Profile: []string{"https://profile-wpa.chat.zalo.me"},
		},
	}

	friends, err := FetchFriends(context.Background(), sess)
	if err != nil {
		t.Fatalf("FetchFriends: %v", err)
	}
	if len(friends) != 2 {
		t.Fatalf("expected 2 friends, got %+v", friends)
	}
	if friends[0].UserID != "user-2" || friends[0].DisplayName != "Bao" || friends[0].ZaloName != "Bao Nguyen" {
		t.Fatalf("unexpected first friend: %+v", friends[0])
	}
	if friends[1].UserID != "user-1" || friends[1].DisplayName != "An" {
		t.Fatalf("unexpected second friend: %+v", friends[1])
	}
}
