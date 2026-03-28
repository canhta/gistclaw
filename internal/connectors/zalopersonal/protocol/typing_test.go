package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func TestSendTypingDM(t *testing.T) {
	keyBytes := []byte("0123456789abcdef0123456789abcdef")
	key := base64.StdEncoding.EncodeToString(keyBytes)

	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", req.Method)
		}
		if req.URL.Path != "/api/message/typing" {
			t.Fatalf("expected /api/message/typing, got %s", req.URL.Path)
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

		plain, err := DecodeAESCBC(keyBytes, params)
		if err != nil {
			t.Fatalf("DecodeAESCBC payload: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(plain, &payload); err != nil {
			t.Fatalf("Unmarshal typing payload: %v", err)
		}
		if payload["toid"] != "user-1" {
			t.Fatalf("expected toid=user-1, got %+v", payload)
		}
		if payload["destType"] != float64(3) {
			t.Fatalf("expected destType=3, got %+v", payload)
		}
		if payload["imei"] != "imei-123" {
			t.Fatalf("expected imei=imei-123, got %+v", payload)
		}
		if _, ok := payload["grid"]; ok {
			t.Fatalf("did not expect grid field for DM typing: %+v", payload)
		}
		return jsonHTTPResponse(t, Response[json.RawMessage]{}), nil
	})
	t.Cleanup(func() { defaultHTTPTransport = oldTransport })

	sess := NewSession()
	sess.IMEI = "imei-123"
	sess.UserAgent = DefaultUserAgent
	sess.SecretKey = key
	sess.LoginInfo = &LoginInfo{
		ZpwServiceMapV3: ZpwServiceMapV3{
			Chat: []string{"https://chat-wpa.chat.zalo.me"},
		},
	}

	if err := SendTyping(context.Background(), sess, "user-1", ThreadTypeUser); err != nil {
		t.Fatalf("SendTyping: %v", err)
	}
}

func TestSendTypingGroup(t *testing.T) {
	keyBytes := []byte("0123456789abcdef0123456789abcdef")
	key := base64.StdEncoding.EncodeToString(keyBytes)

	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", req.Method)
		}
		if req.URL.Path != "/api/group/typing" {
			t.Fatalf("expected /api/group/typing, got %s", req.URL.Path)
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

		plain, err := DecodeAESCBC(keyBytes, params)
		if err != nil {
			t.Fatalf("DecodeAESCBC payload: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(plain, &payload); err != nil {
			t.Fatalf("Unmarshal typing payload: %v", err)
		}
		if payload["grid"] != "group-1" {
			t.Fatalf("expected grid=group-1, got %+v", payload)
		}
		if payload["imei"] != "imei-123" {
			t.Fatalf("expected imei=imei-123, got %+v", payload)
		}
		if _, ok := payload["toid"]; ok {
			t.Fatalf("did not expect toid field for group typing: %+v", payload)
		}
		return jsonHTTPResponse(t, Response[json.RawMessage]{}), nil
	})
	t.Cleanup(func() { defaultHTTPTransport = oldTransport })

	sess := NewSession()
	sess.IMEI = "imei-123"
	sess.UserAgent = DefaultUserAgent
	sess.SecretKey = key
	sess.LoginInfo = &LoginInfo{
		ZpwServiceMapV3: ZpwServiceMapV3{
			Group: []string{"https://group-wpa.chat.zalo.me"},
		},
	}

	if err := SendTyping(context.Background(), sess, "group-1", ThreadTypeGroup); err != nil {
		t.Fatalf("SendTyping: %v", err)
	}
}
