package protocol

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type stubWSClient struct {
	reads  chan []byte
	errors chan error
	writes chan []byte
}

func newStubWSClient() *stubWSClient {
	return &stubWSClient{
		reads:  make(chan []byte, 8),
		errors: make(chan error, 8),
		writes: make(chan []byte, 8),
	}
}

func (c *stubWSClient) ReadMessage(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-c.errors:
		return nil, err
	case msg, ok := <-c.reads:
		if !ok {
			return nil, io.EOF
		}
		return msg, nil
	}
}

func (c *stubWSClient) WriteMessage(_ context.Context, data []byte) error {
	c.writes <- data
	return nil
}

func (c *stubWSClient) Close(int, string) {}

func TestListenerRunEmitsUserMessage(t *testing.T) {
	t.Parallel()

	cipherKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	client := newStubWSClient()
	ln := &Listener{
		sess:           sessWithWSSettings(cipherKey),
		client:         client,
		messageCh:      make(chan Message, 4),
		errorCh:        make(chan error, 4),
		disconnectedCh: make(chan error, 4),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	ln.wg.Add(1)
	go func() {
		defer close(done)
		ln.run(ctx)
	}()

	client.reads <- makeFrame(1, 1, 1, map[string]any{"key": cipherKey})
	client.reads <- makeFrame(1, 501, 0, map[string]any{
		"encrypt": 3,
		"data": encryptWSData(t, cipherKey, map[string]any{
			"data": map[string]any{
				"msgs": []map[string]any{
					{
						"msgId":   "msg-1",
						"uidFrom": "user-1",
						"idTo":    "acct-1",
						"content": "review auth",
					},
				},
			},
		}),
	})

	select {
	case raw := <-ln.messageCh:
		msg, ok := raw.(UserMessage)
		if !ok {
			t.Fatalf("expected UserMessage, got %T", raw)
		}
		if msg.MessageID() != "msg-1" {
			t.Fatalf("expected msg-1, got %q", msg.MessageID())
		}
		if msg.SenderID() != "user-1" {
			t.Fatalf("expected sender user-1, got %q", msg.SenderID())
		}
		if msg.ThreadID() != "user-1" {
			t.Fatalf("expected thread user-1, got %q", msg.ThreadID())
		}
		if msg.Text() != "review auth" {
			t.Fatalf("expected text review auth, got %q", msg.Text())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for listener message")
	}

	cancel()
	close(client.reads)
	<-done
}

func TestListenerRunEmitsGroupMessage(t *testing.T) {
	t.Parallel()

	cipherKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	client := newStubWSClient()
	ln := &Listener{
		sess:           sessWithWSSettings(cipherKey),
		client:         client,
		messageCh:      make(chan Message, 4),
		errorCh:        make(chan error, 4),
		disconnectedCh: make(chan error, 4),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	ln.wg.Add(1)
	go func() {
		defer close(done)
		ln.run(ctx)
	}()

	client.reads <- makeFrame(1, 1, 1, map[string]any{"key": cipherKey})
	client.reads <- makeFrame(1, 521, 0, map[string]any{
		"encrypt": 3,
		"data": encryptWSData(t, cipherKey, map[string]any{
			"data": map[string]any{
				"groupMsgs": []map[string]any{
					{
						"msgId":   "group-msg-1",
						"uidFrom": "user-2",
						"idTo":    "group-1",
						"content": "@acct-1 review this",
						"mentions": []map[string]any{
							{"uid": "acct-1", "pos": 0, "len": 7, "type": 0},
						},
					},
				},
			},
		}),
	})

	select {
	case raw := <-ln.messageCh:
		msg, ok := raw.(GroupMessage)
		if !ok {
			t.Fatalf("expected GroupMessage, got %T", raw)
		}
		if msg.MessageID() != "group-msg-1" {
			t.Fatalf("expected group-msg-1, got %q", msg.MessageID())
		}
		if msg.ThreadID() != "group-1" {
			t.Fatalf("expected thread group-1, got %q", msg.ThreadID())
		}
		if !msg.MentionsAccount("acct-1") {
			t.Fatalf("expected mention to be preserved, got %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for group listener message")
	}

	cancel()
	close(client.reads)
	<-done
}

func TestListenerHandleControlEventResolvesUploadCallback(t *testing.T) {
	t.Parallel()

	cipherKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	ln := &Listener{
		sess:           sessWithWSSettings(cipherKey),
		messageCh:      make(chan Message, 1),
		errorCh:        make(chan error, 1),
		disconnectedCh: make(chan error, 1),
	}
	ln.cipherKey = cipherKey

	urlCh := ln.RegisterUploadCallback("123")
	frame := makeFrame(1, 601, 0, map[string]any{
		"encrypt": 3,
		"data": encryptWSData(t, cipherKey, map[string]any{
			"data": map[string]any{
				"controls": []map[string]any{
					{
						"content": map[string]any{
							"act_type": "file_done",
							"fileId":   123,
							"data": map[string]any{
								"url": "https://example.com/report.pdf",
							},
						},
					},
				},
			},
		}),
	})
	if err := ln.handleFrame(context.Background(), frame); err != nil {
		t.Fatalf("handleFrame: %v", err)
	}

	select {
	case fileURL := <-urlCh:
		if fileURL != "https://example.com/report.pdf" {
			t.Fatalf("expected callback URL, got %q", fileURL)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upload callback URL")
	}

	if _, ok := ln.uploadCallbacks.Load("123"); ok {
		t.Fatal("expected upload callback to be removed after file_done")
	}
}

func sessWithWSSettings(cipherKey string) *Session {
	sess := NewSession()
	sess.UID = "acct-1"
	sess.SecretKey = cipherKey
	sess.Settings = &Settings{
		Features: Features{
			Socket: SocketSettings{
				PingInterval: 10000,
			},
		},
	}
	return sess
}

func makeFrame(version uint8, cmd uint16, subCmd uint8, payload map[string]any) []byte {
	body, _ := json.Marshal(payload)
	frame := make([]byte, 4+len(body))
	frame[0] = version
	binary.LittleEndian.PutUint16(frame[1:3], cmd)
	frame[3] = subCmd
	copy(frame[4:], body)
	return frame
}

func encryptWSData(t *testing.T, cipherKey string, payload map[string]any) string {
	t.Helper()

	key, err := base64.StdEncoding.DecodeString(cipherKey)
	if err != nil {
		t.Fatalf("DecodeString cipher key: %v", err)
	}
	plain, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, 16)
	if err != nil {
		t.Fatalf("NewGCMWithNonceSize: %v", err)
	}
	iv := []byte("1234567890abcdef")
	aad := []byte("fedcba0987654321")
	ciphertext := gcm.Seal(nil, iv, plain, aad)
	raw := append(append(append([]byte{}, iv...), aad...), ciphertext...)
	return base64.StdEncoding.EncodeToString(raw)
}

func TestNewListenerRejectsMissingWSURL(t *testing.T) {
	t.Parallel()

	_, err := NewListener(&Session{})
	if err == nil {
		t.Fatal("expected missing websocket URL error")
	}
}

func TestListenerRetriesTransientDisconnectAndRotatesEndpoint(t *testing.T) {
	t.Parallel()

	cipherKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	sess := sessWithWSSettings(cipherKey)
	sess.LoginInfo = &LoginInfo{
		ZpwWebsocket: []string{
			"wss://ws-1.example.test/socket",
			"wss://ws-2.example.test/socket",
		},
	}
	maxRetries := 1
	sess.Settings.Features.Socket.Retries = map[string]SocketRetryConfig{
		"1006": {
			Max:   &maxRetries,
			Times: []int{1},
		},
	}
	sess.Settings.Features.Socket.RotateErrorCodes = []int{1006}

	ln, err := NewListener(sess)
	if err != nil {
		t.Fatalf("NewListener: %v", err)
	}

	first := newStubWSClient()
	second := newStubWSClient()
	dialed := make([]string, 0, 2)
	ln.dialWS = func(_ context.Context, wsURL string, _ http.Header, _ http.CookieJar) (wsClient, error) {
		dialed = append(dialed, wsURL)
		if len(dialed) == 1 {
			return first, nil
		}
		return second, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ln.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ln.Stop()

	first.errors <- &websocket.CloseError{Code: 1006, Text: "transient disconnect"}
	second.reads <- makeFrame(1, 1, 1, map[string]any{"key": cipherKey})
	second.reads <- makeFrame(1, 501, 0, map[string]any{
		"encrypt": 3,
		"data": encryptWSData(t, cipherKey, map[string]any{
			"data": map[string]any{
				"msgs": []map[string]any{
					{
						"msgId":   "msg-rotated",
						"uidFrom": "user-2",
						"idTo":    "acct-1",
						"content": "after retry",
					},
				},
			},
		}),
	})

	select {
	case raw := <-ln.Messages():
		msg, ok := raw.(UserMessage)
		if !ok {
			t.Fatalf("expected UserMessage, got %T", raw)
		}
		if msg.MessageID() != "msg-rotated" {
			t.Fatalf("expected retried message, got %q", msg.MessageID())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for retried listener message")
	}

	if len(dialed) < 2 {
		t.Fatalf("expected listener to dial twice, got %d dials", len(dialed))
	}
	if dialed[0] == dialed[1] {
		t.Fatalf("expected endpoint rotation, got identical URLs %q", dialed[0])
	}
}

var _ wsClient = (*stubWSClient)(nil)
var _ http.RoundTripper = roundTripFunc(nil)
