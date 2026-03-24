package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBot_NoTokenDoesNotStart(t *testing.T) {
	// When no token is configured, Start must return immediately without error
	// and without spawning a goroutine that calls getUpdates.
	bot := NewBot("", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := bot.Start(ctx); err != nil {
		t.Fatalf("Start with empty token: expected nil, got %v", err)
	}
	// No API calls should have been made (verified implicitly by no server needed).
}

func TestBot_WithTokenCallsGetUpdates(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "getUpdates") {
			callCount.Add(1)
			// Return empty updates so the bot doesn't try to dispatch.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
		}
	}))
	defer srv.Close()

	bot := NewBot("testtoken123", nil)
	bot.apiBase = srv.URL + "/bot"

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = bot.Start(ctx)

	if callCount.Load() == 0 {
		t.Fatal("expected at least one getUpdates call when token is configured")
	}
}

func TestBot_ContextCancelExitsLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer srv.Close()

	bot := NewBot("testtoken", nil)
	bot.apiBase = srv.URL + "/bot"

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		_ = bot.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// good — bot exited after context cancel
	case <-time.After(5 * time.Second):
		t.Fatal("bot did not exit within 5 seconds after context cancel")
	}
}

func TestBot_DispatchesUpdatesToHandler(t *testing.T) {
	var dispatched atomic.Int32

	handler := func(upd Update) {
		dispatched.Add(1)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "getUpdates") {
			w.Header().Set("Content-Type", "application/json")
			// Return one update, then empty to let the loop run once more before cancel.
			if dispatched.Load() == 0 {
				_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":1,"message":{"message_id":1,"chat":{"id":42,"type":"private"},"text":"hello"}}]}`))
			} else {
				_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
			}
		}
	}))
	defer srv.Close()

	bot := NewBot("testtoken", handler)
	bot.apiBase = srv.URL + "/bot"

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = bot.Start(ctx)

	if dispatched.Load() == 0 {
		t.Fatal("expected at least one update dispatched to handler")
	}
}

func TestBot_NoWebhookRegistered(t *testing.T) {
	// The connector must never call setWebhook — long polling only.
	var webhookCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "setWebhook") {
			webhookCalled = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer srv.Close()

	bot := NewBot("testtoken", nil)
	bot.apiBase = srv.URL + "/bot"

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = bot.Start(ctx)

	if webhookCalled {
		t.Fatal("bot must not register a webhook endpoint")
	}
}
