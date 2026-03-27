package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSendMessageDM(t *testing.T) {
	t.Parallel()

	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", req.Method)
		}
		if req.URL.Path != "/api/message/sms" {
			t.Fatalf("expected /api/message/sms, got %s", req.URL.Path)
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
			t.Fatalf("Unmarshal send payload: %v", err)
		}
		if payload["message"] != "xin chao" {
			t.Fatalf("expected message payload, got %+v", payload)
		}
		if payload["toid"] != "user-1" {
			t.Fatalf("expected toid user-1, got %+v", payload)
		}

		inner, err := json.Marshal(Response[json.RawMessage]{
			Data: json.RawMessage(`{"msgId":"msg-123"}`),
		})
		if err != nil {
			t.Fatalf("Marshal inner response: %v", err)
		}
		encrypted, err := EncodeAESCBC([]byte("0123456789abcdef0123456789abcdef"), string(inner), false)
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
			Chat: []string{"https://chat-wpa.chat.zalo.me"},
		},
	}

	msgID, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "xin chao")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if msgID != "msg-123" {
		t.Fatalf("expected msg-123, got %q", msgID)
	}
}

func TestSendMessageDMAcceptsNumericMessageID(t *testing.T) {
	t.Parallel()

	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		inner, err := json.Marshal(Response[json.RawMessage]{
			Data: json.RawMessage(`{"msgId":123456789}`),
		})
		if err != nil {
			t.Fatalf("Marshal inner response: %v", err)
		}
		encrypted, err := EncodeAESCBC([]byte("0123456789abcdef0123456789abcdef"), string(inner), false)
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
			Chat: []string{"https://chat-wpa.chat.zalo.me"},
		},
	}

	msgID, err := SendMessage(context.Background(), sess, "user-1", ThreadTypeUser, "xin chao")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if msgID != "123456789" {
		t.Fatalf("expected numeric msg ID to round-trip as string, got %q", msgID)
	}
}

func ioReadAll(rc interface{ Read([]byte) (int, error) }) ([]byte, error) {
	buf := make([]byte, 0, 256)
	tmp := make([]byte, 256)
	for {
		n, err := rc.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				return buf, nil
			}
			return buf, err
		}
	}
}
