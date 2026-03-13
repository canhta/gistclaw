package claudecode

import (
	"encoding/json"
	"fmt"
	"strings"
)

// StreamEvent is the parsed form of a single newline-delimited JSON object from
// the `claude -p --output-format stream-json` stdout.
//
// Relevant types:
//   - "text"   — Text field holds the incremental content.
//   - "result" — TotalCostUSD and IsError hold the final cost and error status.
//   - All other types (e.g. "assistant", "user", "system") are passed through
//     unchanged; the caller may ignore unknown types.
type StreamEvent struct {
	Type         string  `json:"type"`
	Text         string  `json:"text"`           // "text"
	TotalCostUSD float64 `json:"total_cost_usd"` // "result"
	IsError      bool    `json:"is_error"`       // "result"
	Result       string  `json:"result"`         // "result" — final message or error detail
}

// ParseStreamLine parses a single line from `claude -p` stream-json stdout.
//
// Rules:
//   - Empty lines return (nil, nil) — callers must handle this gracefully.
//   - Non-empty lines must be valid JSON; malformed JSON returns an error.
//     The caller should log the error as WARN and skip (do NOT crash).
func ParseStreamLine(line string) (*StreamEvent, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}
	var ev StreamEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return nil, fmt.Errorf("stream-json parse error: %w (line: %q)", err, line)
	}
	return &ev, nil
}
