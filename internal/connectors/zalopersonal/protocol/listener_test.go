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
)

type stubWSClient struct {
	reads  chan []byte
	writes chan []byte
}

func newStubWSClient() *stubWSClient {
	return &stubWSClient{
		reads:  make(chan []byte, 8),
		writes: make(chan []byte, 8),
	}
}

func (c *stubWSClient) ReadMessage(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
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

var _ wsClient = (*stubWSClient)(nil)
var _ http.RoundTripper = roundTripFunc(nil)
