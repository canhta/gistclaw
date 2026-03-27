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

type stubUploadCallbacks struct {
	chans      map[string]chan string
	registered chan string
}

func (s *stubUploadCallbacks) RegisterUploadCallback(fileID string) <-chan string {
	if s.chans == nil {
		s.chans = make(map[string]chan string)
	}
	ch := make(chan string, 1)
	s.chans[fileID] = ch
	if s.registered != nil {
		s.registered <- fileID
	}
	return ch
}

func (s *stubUploadCallbacks) CancelUploadCallback(fileID string) {
	delete(s.chans, fileID)
}

func TestUploadFileWaitsForCallbackURL(t *testing.T) {
	keyBytes := []byte("0123456789abcdef0123456789abcdef")
	key := base64.StdEncoding.EncodeToString(keyBytes)

	callbacks := &stubUploadCallbacks{registered: make(chan string, 1)}
	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/message/asyncfile/upload" {
			t.Fatalf("expected async file upload path, got %s", req.URL.Path)
		}

		inner, err := json.Marshal(Response[json.RawMessage]{
			Data: json.RawMessage(`{
				"fileId":"file-123",
				"clientFileId":"client-123",
				"chunkId":1,
				"finished":1
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

		go func() {
			fileID := <-callbacks.registered
			callbacks.chans[fileID] <- "https://example.com/report.pdf"
		}()

		return jsonHTTPResponse(t, Response[*string]{Data: &escaped}), nil
	})
	t.Cleanup(func() { defaultHTTPTransport = oldTransport })

	filePath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
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

	result, err := UploadFile(context.Background(), sess, callbacks, "user-1", ThreadTypeUser, filePath)
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if result.FileID != "file-123" || result.FileURL != "https://example.com/report.pdf" {
		t.Fatalf("unexpected upload result: %+v", result)
	}
}

func TestSendFileDM(t *testing.T) {
	keyBytes := []byte("0123456789abcdef0123456789abcdef")
	key := base64.StdEncoding.EncodeToString(keyBytes)

	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", req.Method)
		}
		if req.URL.Path != "/api/message/asyncfile/msg" {
			t.Fatalf("expected async file send path, got %s", req.URL.Path)
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
		if payload["fileId"] != "file-123" {
			t.Fatalf("expected fileId file-123, got %+v", payload)
		}
		if payload["fileUrl"] != "https://example.com/report.pdf" {
			t.Fatalf("expected fileUrl in payload, got %+v", payload)
		}

		inner, err := json.Marshal(Response[json.RawMessage]{
			Data: json.RawMessage(`{"msgId":"file-msg-1"}`),
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

	msgID, err := SendFile(context.Background(), sess, "user-1", ThreadTypeUser, &FileUploadResult{
		FileID:       "file-123",
		FileURL:      "https://example.com/report.pdf",
		ClientFileID: "client-123",
		TotalSize:    5,
		FileName:     "report.txt",
		Checksum:     "5d41402abc4b2a76b9719d911017c592",
	})
	if err != nil {
		t.Fatalf("SendFile: %v", err)
	}
	if msgID != "file-msg-1" {
		t.Fatalf("expected file-msg-1, got %q", msgID)
	}
}
