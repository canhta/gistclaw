package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadImage(t *testing.T) {
	keyBytes := []byte("0123456789abcdef0123456789abcdef")
	key := base64.StdEncoding.EncodeToString(keyBytes)

	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", req.Method)
		}
		if req.URL.Path != "/api/message/photo_original/upload" {
			t.Fatalf("expected image upload path, got %s", req.URL.Path)
		}

		inner, err := json.Marshal(Response[json.RawMessage]{
			Data: json.RawMessage(`{
				"normalUrl":"https://example.com/image.png",
				"hdUrl":"https://example.com/image-hd.png",
				"thumbUrl":"https://example.com/thumb.png",
				"photoId":"photo-123",
				"clientFileId":"client-123",
				"chunkId":1,
				"finished":true
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

	filePath := filepath.Join(t.TempDir(), "tiny.png")
	if err := os.WriteFile(filePath, tinyPNG1x1, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sess := NewSession()
	sess.IMEI = "imei-123"
	sess.UserAgent = DefaultUserAgent
	sess.SecretKey = key
	sess.LoginInfo = &LoginInfo{
		ZpwServiceMapV3: ZpwServiceMapV3{
			File: []string{"https://file-wpa.chat.zalo.me"},
		},
	}

	result, err := UploadImage(context.Background(), sess, "user-1", ThreadTypeUser, filePath)
	if err != nil {
		t.Fatalf("UploadImage: %v", err)
	}
	if result.PhotoID.String() != "photo-123" || result.ClientFileID.String() != "client-123" {
		t.Fatalf("unexpected upload result: %+v", result)
	}
	if result.Width != 1 || result.Height != 1 {
		t.Fatalf("expected 1x1 image dimensions, got %+v", result)
	}
}

func TestSendImageDM(t *testing.T) {
	keyBytes := []byte("0123456789abcdef0123456789abcdef")
	key := base64.StdEncoding.EncodeToString(keyBytes)

	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", req.Method)
		}
		if req.URL.Path != "/api/message/photo_original/send" {
			t.Fatalf("expected image send path, got %s", req.URL.Path)
		}

		bodyBytes, err := ioReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll body: %v", err)
		}
		values, err := url.ParseQuery(string(bodyBytes))
		if err != nil {
			t.Fatalf("ParseQuery: %v", err)
		}

		plain, err := DecodeAESCBC(keyBytes, values.Get("params"))
		if err != nil {
			t.Fatalf("DecodeAESCBC payload: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(plain, &payload); err != nil {
			t.Fatalf("Unmarshal send payload: %v", err)
		}
		if payload["toid"] != "user-1" {
			t.Fatalf("expected toid user-1, got %+v", payload)
		}
		if payload["desc"] != "release evidence" {
			t.Fatalf("expected caption in payload, got %+v", payload)
		}
		if payload["photoId"] != "photo-123" {
			t.Fatalf("expected photoId photo-123, got %+v", payload)
		}

		inner, err := json.Marshal(Response[json.RawMessage]{
			Data: json.RawMessage(`{"msgId":"img-msg-1"}`),
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
			File: []string{"https://file-wpa.chat.zalo.me"},
		},
	}

	msgID, err := SendImage(context.Background(), sess, "user-1", ThreadTypeUser, &ImageUploadResult{
		NormalURL:    "https://example.com/image.png",
		HDUrl:        "https://example.com/image-hd.png",
		ThumbURL:     "https://example.com/thumb.png",
		PhotoID:      "photo-123",
		Width:        1,
		Height:       1,
		TotalSize:    67,
		Finished:     true,
		ClientFileID: "client-123",
	}, "release evidence")
	if err != nil {
		t.Fatalf("SendImage: %v", err)
	}
	if msgID != "img-msg-1" {
		t.Fatalf("expected img-msg-1, got %q", msgID)
	}
}

func TestIsImageFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{name: "png", path: "/tmp/photo.png", expected: true},
		{name: "jpeg upper-case", path: "/tmp/PHOTO.JPEG", expected: true},
		{name: "webp", path: "/tmp/photo.webp", expected: true},
		{name: "text", path: "/tmp/report.txt", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsImageFile(tt.path); got != tt.expected {
				t.Fatalf("IsImageFile(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

var tinyPNG1x1 = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
	0x00, 0x03, 0x01, 0x01, 0x00, 0xc9, 0xfe, 0x92,
	0xef, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
	0x44, 0xae, 0x42, 0x60, 0x82,
}
