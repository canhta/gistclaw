// internal/channel/channel_test.go
package channel_test

import (
	"context"
	"testing"

	"github.com/canhta/gistclaw/internal/channel"
)

// mockChannel implements channel.Channel for compile-time interface verification.
type mockChannel struct{}

func (m *mockChannel) Receive(_ context.Context) (<-chan channel.InboundMessage, error) {
	return nil, nil
}
func (m *mockChannel) SendMessage(_ context.Context, _ int64, _ string) error { return nil }
func (m *mockChannel) SendKeyboard(_ context.Context, _ int64, _ channel.KeyboardPayload) error {
	return nil
}
func (m *mockChannel) SendTyping(_ context.Context, _ int64) error { return nil }
func (m *mockChannel) Name() string                                { return "mock" }

// Compile-time assertion: mockChannel must satisfy channel.Channel.
// If the interface changes and mockChannel is not updated, this line fails to compile.
var _ channel.Channel = (*mockChannel)(nil)

func TestKeyboardPayloadHasRows(t *testing.T) {
	payload := channel.KeyboardPayload{
		Text: "Choose:",
		Rows: []channel.ButtonRow{
			{
				{Label: "Yes", CallbackData: "yes"},
				{Label: "No", CallbackData: "no"},
			},
		},
	}
	if len(payload.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(payload.Rows))
	}
	if len(payload.Rows[0]) != 2 {
		t.Errorf("expected 2 buttons, got %d", len(payload.Rows[0]))
	}
	if payload.Rows[0][0].Label != "Yes" {
		t.Errorf("expected label 'Yes', got %q", payload.Rows[0][0].Label)
	}
}

func TestInboundMessageZeroValue(t *testing.T) {
	var msg channel.InboundMessage
	if msg.ChatID != 0 {
		t.Error("zero value ChatID should be 0")
	}
	if msg.CallbackData != "" {
		t.Error("zero value CallbackData should be empty")
	}
}
