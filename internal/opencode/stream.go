// internal/opencode/stream.go
package opencode

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SSEOption is a single selectable answer in a Question received via SSE.
type SSEOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// SSEQuestion is a single question within a question.asked SSE event.
type SSEQuestion struct {
	Question string      `json:"question"`
	Header   string      `json:"header"`
	Options  []SSEOption `json:"options"`
	Multiple bool        `json:"multiple"`
	Custom   bool        `json:"custom"`
}

// SSEPart is the part payload inside a message.part.updated event.
type SSEPart struct {
	// Type is "text" or "step-finish" (and potentially others — unknown types are ignored).
	Type    string  `json:"type"`
	Text    string  `json:"text"`     // set when Type == "text"
	CostUSD float64 `json:"cost_usd"` // set when Type == "step-finish"
}

// SSEStatus is the status payload inside a session.status event.
type SSEStatus struct {
	// Type is "idle", "running", etc.
	Type string `json:"type"`
}

// SSEEvent is the unified parsed form of a single SSE data line from OpenCode.
// Only the fields relevant to the observed event type are populated.
type SSEEvent struct {
	// Common fields
	Type      string `json:"type"`
	ID        string `json:"id"`         // permission.asked / question.asked
	SessionID string `json:"session_id"` // permission.asked / question.asked

	// message.part.updated
	Part *SSEPart `json:"part,omitempty"`

	// permission.asked
	Permission string   `json:"permission,omitempty"`
	Patterns   []string `json:"patterns,omitempty"`

	// question.asked
	Questions []SSEQuestion `json:"questions,omitempty"`

	// session.status
	Status *SSEStatus `json:"status,omitempty"`
}

// ParseSSELine parses a single line from an OpenCode SSE stream.
//
// Rules:
//   - Lines not starting with "data: " are silently skipped (returns nil, nil).
//     This includes blank lines, comment lines (": ..."), and "event:" lines.
//   - Lines starting with "data: " must contain valid JSON; malformed JSON returns an error.
//     The caller should log the error as WARN and skip the line rather than crashing.
func ParseSSELine(line string) (*SSEEvent, error) {
	const prefix = "data: "
	if !strings.HasPrefix(line, prefix) {
		return nil, nil
	}
	payload := line[len(prefix):]
	var ev SSEEvent
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		return nil, fmt.Errorf("SSE JSON parse error: %w (line: %q)", err, line)
	}
	return &ev, nil
}
