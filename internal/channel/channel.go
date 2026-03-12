// internal/channel/channel.go
package channel

import "context"

// InboundMessage is a platform-agnostic inbound event.
// For Telegram: text messages and inline keyboard callback queries both arrive as InboundMessage.
type InboundMessage struct {
	ID     string // platform-specific message ID (string for cross-platform compatibility)
	ChatID int64
	UserID int64
	Text   string
	// CallbackData is non-empty for inline keyboard button presses.
	// Empty for plain text messages.
	CallbackData string
}

// Button is a single button in an inline keyboard row.
type Button struct {
	Label        string
	CallbackData string
}

// ButtonRow is one horizontal row of buttons.
type ButtonRow []Button

// KeyboardPayload is a platform-agnostic keyboard definition.
// hitl/keyboard.go constructs this type.
// Channel adapters (e.g. channel/telegram) translate it to platform-specific types.
// hitl must NOT import any platform-specific package (e.g. telego).
type KeyboardPayload struct {
	Text string      // message text displayed above the keyboard
	Rows []ButtonRow // rows of buttons; each row is displayed on one line
}

// Channel is the platform abstraction for sending and receiving messages.
// v1 implementation: internal/channel/telegram.
// Future: internal/channel/whatsapp, etc.
//
// Adding a new channel:
//  1. Implement this interface in internal/channel/<name>/<name>.go
//  2. Add a case to app.NewApp channel factory switch (CHANNEL env var)
//  3. gateway.Service does not change — it only uses this interface
type Channel interface {
	// Receive returns a channel of inbound messages. Runs until ctx is cancelled.
	// Post-v1 consideration: split into Start(ctx) error + Receive() <-chan InboundMessage
	// if startup errors need to be separated from runtime errors.
	Receive(ctx context.Context) (<-chan InboundMessage, error)

	SendMessage(ctx context.Context, chatID int64, text string) error
	SendKeyboard(ctx context.Context, chatID int64, payload KeyboardPayload) error
	SendTyping(ctx context.Context, chatID int64) error

	// Name returns the platform identifier, e.g. "telegram", "whatsapp".
	Name() string
}
