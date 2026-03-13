// internal/channel/telegram/telegram_test.go
package telegram_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canhta/gistclaw/internal/channel"
	tgchan "github.com/canhta/gistclaw/internal/channel/telegram"
	"github.com/canhta/gistclaw/internal/store"
	telego "github.com/mymmrac/telego"
)

// validToken is a token that passes telego's format validation: `^\d+:[\w-]{35}$`
const validToken = "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghi"

// newTestStore creates an in-memory SQLite store for tests.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// fakeTelegramServer returns an httptest.Server that responds to Telegram Bot API requests.
// It records calls to /bot<token>/sendMessage.
type fakeTelegramServer struct {
	server       *httptest.Server
	sentMessages []string // captured text payloads
	callCount    atomic.Int32
}

func newFakeTelegramServer(t *testing.T) *fakeTelegramServer {
	t.Helper()
	fake := &fakeTelegramServer{}
	mux := http.NewServeMux()

	// getMe — required by telego on startup
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "getMe") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"id": 1, "is_bot": true, "first_name": "TestBot", "username": "testbot"},
			})
			return
		}
		if strings.Contains(path, "getUpdates") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []any{}})
			return
		}
		if strings.Contains(path, "sendMessage") {
			fake.callCount.Add(1)
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if text, ok := body["text"].(string); ok {
				fake.sentMessages = append(fake.sentMessages, text)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"message_id": 1, "date": 1, "chat": map[string]any{"id": 123}},
			})
			return
		}
		if strings.Contains(path, "sendChatAction") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
			return
		}
		// Default: 404
		http.NotFound(w, r)
	})

	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

// Verify TelegramChannel satisfies channel.Channel at compile time.
var _ channel.Channel = (*tgchan.TelegramChannel)(nil)

func TestTelegramChannelName(t *testing.T) {
	fake := newFakeTelegramServer(t)
	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL(validToken, s, fake.server.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}
	if ch.Name() != "telegram" {
		t.Errorf("Name() = %q, want telegram", ch.Name())
	}
}

func TestSendMessageShortText(t *testing.T) {
	fake := newFakeTelegramServer(t)
	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL(validToken, s, fake.server.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}
	ctx := context.Background()
	if err := ch.SendMessage(ctx, 123, "hello"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if fake.callCount.Load() != 1 {
		t.Errorf("expected 1 sendMessage call, got %d", fake.callCount.Load())
	}
}

func TestSendMessageSplitsLongText(t *testing.T) {
	fake := newFakeTelegramServer(t)
	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL(validToken, s, fake.server.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}
	// Build a string > 4096 chars; must be split into 2 messages.
	longText := strings.Repeat("a", 5000)
	ctx := context.Background()
	if err := ch.SendMessage(ctx, 123, longText); err != nil {
		t.Fatalf("SendMessage long: %v", err)
	}
	if fake.callCount.Load() != 2 {
		t.Errorf("expected 2 sendMessage calls for 5000-char text, got %d", fake.callCount.Load())
	}
}

func TestSendMessageSplitsExactlyAtTelegramLimit(t *testing.T) {
	fake := newFakeTelegramServer(t)
	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL(validToken, s, fake.server.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}
	// telegramLimit = 4096-32 = 4064; a message exactly at this size fits in one send.
	exactText := strings.Repeat("b", tgchan.TelegramLimit)
	ctx := context.Background()
	if err := ch.SendMessage(ctx, 123, exactText); err != nil {
		t.Fatalf("SendMessage exact telegramLimit: %v", err)
	}
	if fake.callCount.Load() != 1 {
		t.Errorf("expected 1 sendMessage call for %d-char text, got %d", tgchan.TelegramLimit, fake.callCount.Load())
	}
}

func TestSendKeyboardTranslatesPayload(t *testing.T) {
	// Track the full request body to verify inline keyboard structure.
	var capturedBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "getMe") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"id": 1, "is_bot": true, "first_name": "TestBot", "username": "testbot"},
			})
			return
		}
		if strings.Contains(path, "sendMessage") {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"message_id": 1, "date": 1, "chat": map[string]any{"id": 123}},
			})
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL(validToken, s, srv.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}

	payload := channel.KeyboardPayload{
		Text: "Choose an action:",
		Rows: []channel.ButtonRow{
			{
				{Label: "✅ Once", CallbackData: "hitl:perm_001:once"},
				{Label: "❌ Reject", CallbackData: "hitl:perm_001:reject"},
			},
		},
	}

	ctx := context.Background()
	if err := ch.SendKeyboard(ctx, 123, payload); err != nil {
		t.Fatalf("SendKeyboard: %v", err)
	}

	// Verify text was sent.
	if capturedBody["text"] != "Choose an action:" {
		t.Errorf("text = %v, want 'Choose an action:'", capturedBody["text"])
	}

	// Verify reply_markup was set.
	markup, ok := capturedBody["reply_markup"].(map[string]any)
	if !ok {
		t.Fatalf("reply_markup missing or wrong type: %T", capturedBody["reply_markup"])
	}
	inlineKeyboard, ok := markup["inline_keyboard"].([]any)
	if !ok {
		t.Fatalf("inline_keyboard missing: %v", markup)
	}
	if len(inlineKeyboard) != 1 {
		t.Fatalf("expected 1 keyboard row, got %d", len(inlineKeyboard))
	}
	row, ok := inlineKeyboard[0].([]any)
	if !ok {
		t.Fatalf("row 0 wrong type: %T", inlineKeyboard[0])
	}
	if len(row) != 2 {
		t.Fatalf("row 0: expected 2 buttons, got %d", len(row))
	}
}

func TestSendTyping(t *testing.T) {
	var typingCalled atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "getMe") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"id": 1, "is_bot": true, "first_name": "TestBot", "username": "testbot"},
			})
			return
		}
		if strings.Contains(path, "sendChatAction") {
			typingCalled.Store(true)
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL(validToken, s, srv.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}
	ctx := context.Background()
	if err := ch.SendTyping(ctx, 123); err != nil {
		t.Fatalf("SendTyping: %v", err)
	}
	if !typingCalled.Load() {
		t.Error("expected sendChatAction to be called")
	}
}

func TestReceiveContextCancellation(t *testing.T) {
	fake := newFakeTelegramServer(t)
	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL(validToken, s, fake.server.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	msgs, err := ch.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	// Drain until context cancels; should not block forever.
	for {
		select {
		case <-msgs:
		case <-ctx.Done():
			return // success
		}
	}
}

// TestUpdateIDStored verifies that SetLastUpdateID is called and GetLastUpdateID returns the right value.
func TestUpdateIDStored(t *testing.T) {
	s := newTestStore(t)

	// Initially no record for this channel.
	id, err := s.GetLastUpdateID("telegram:testbot")
	if err != nil {
		t.Fatalf("GetLastUpdateID: %v", err)
	}
	if id != 0 {
		t.Errorf("initial update ID: got %d, want 0", id)
	}

	// Simulate storing an update ID (as TelegramChannel would do).
	if err := s.SetLastUpdateID("telegram:testbot", 99); err != nil {
		t.Fatalf("SetLastUpdateID: %v", err)
	}
	id, err = s.GetLastUpdateID("telegram:testbot")
	if err != nil {
		t.Fatalf("GetLastUpdateID after set: %v", err)
	}
	if id != 99 {
		t.Errorf("after set: got %d, want 99", id)
	}
}

func TestNewTelegramChannelConstructor(t *testing.T) {
	s := newTestStore(t)
	fake := newFakeTelegramServer(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL(validToken, s, fake.server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil TelegramChannel")
	}
}

// TestSendMessageRetriesNetworkError verifies that a transient network error
// is retried up to 3 times before returning an error, per §9.21.
func TestSendMessageRetriesNetworkError(t *testing.T) {
	var callCount atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "getMe") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"id": 1, "is_bot": true, "first_name": "TestBot", "username": "testbot"},
			})
			return
		}
		if strings.Contains(path, "sendMessage") {
			// Simulate a network error by closing the connection immediately.
			callCount.Add(1)
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("ResponseWriter does not implement Hijacker")
				return
			}
			conn, _, _ := hj.Hijack()
			conn.Close() // causes EOF on the client side
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := newTestStore(t)
	ch, err := tgchan.NewTelegramChannelWithBaseURL(validToken, s, srv.URL)
	if err != nil {
		t.Fatalf("NewTelegramChannelWithBaseURL: %v", err)
	}

	ctx := context.Background()
	err = ch.SendMessage(ctx, 123, "hello")
	if err == nil {
		t.Fatal("expected error after exhausted network retries, got nil")
	}
	// Expect 4 total attempts: initial + 3 retries.
	if got := callCount.Load(); got != 4 {
		t.Errorf("expected 4 sendMessage attempts (1 initial + 3 retries), got %d", got)
	}
}

// Ensure TelegramChannel's unused import of telego compiles correctly.
var _ *telego.Bot // reference telego to confirm it's imported in the test binary
