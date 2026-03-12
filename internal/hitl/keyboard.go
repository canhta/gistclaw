// internal/hitl/keyboard.go
package hitl

// IMPORTANT: This file must ONLY import internal/channel.
// No telego, no external dependencies. The Telegram adapter in
// internal/channel/telegram translates channel.KeyboardPayload to telego types.

import (
	"fmt"
	"strings"

	"github.com/canhta/gistclaw/internal/channel"
)

// PermissionKeyboard builds the keyboard for a permission approval request.
//
// Text format:
//
//	🔐 Permission request
//	<permission> on:
//	<pattern1>
//	<pattern2>
//
// Rows:
//
//	[✅ Once]    → "hitl:<id>:once"
//	[✅ Always]  → "hitl:<id>:always"
//	[❌ Reject]  → "hitl:<id>:reject"
//	[⏹ Stop]    → "hitl:<id>:stop"
func PermissionKeyboard(id, permission string, patterns []string) channel.KeyboardPayload {
	var sb strings.Builder
	sb.WriteString("🔐 Permission request\n")
	sb.WriteString(permission)
	sb.WriteString(" on:")
	for _, p := range patterns {
		sb.WriteByte('\n')
		sb.WriteString(p)
	}

	return channel.KeyboardPayload{
		Text: sb.String(),
		Rows: []channel.ButtonRow{
			{{Label: "✅ Once", CallbackData: fmt.Sprintf("hitl:%s:once", id)}},
			{{Label: "✅ Always", CallbackData: fmt.Sprintf("hitl:%s:always", id)}},
			{{Label: "❌ Reject", CallbackData: fmt.Sprintf("hitl:%s:reject", id)}},
			{{Label: "⏹ Stop", CallbackData: fmt.Sprintf("hitl:%s:stop", id)}},
		},
	}
}

// QuestionKeyboard builds the keyboard for a single Question within a QuestionRequest.
//
// Text: the question text verbatim.
// Rows: one button per option (on its own row).
//
// CallbackData format for options: "hitl:<id>:opt:<index>"
// CallbackData for custom text:    "hitl:<id>:custom"
//
// If q.Custom is true, an extra "✏️ Type your own" button is appended as the last row.
//
// Option button labels:
//   - If Option.Description is non-empty: "<Label> — <Description>"
//   - Otherwise: "<Label>"
func QuestionKeyboard(id string, q Question) channel.KeyboardPayload {
	rows := make([]channel.ButtonRow, 0, len(q.Options)+1)

	for i, opt := range q.Options {
		label := opt.Label
		if opt.Description != "" {
			label = opt.Label + " — " + opt.Description
		}
		rows = append(rows, channel.ButtonRow{
			{
				Label:        label,
				CallbackData: fmt.Sprintf("hitl:%s:opt:%d", id, i),
			},
		})
	}

	if q.Custom {
		rows = append(rows, channel.ButtonRow{
			{
				Label:        "✏️ Type your own",
				CallbackData: fmt.Sprintf("hitl:%s:custom", id),
			},
		})
	}

	return channel.KeyboardPayload{
		Text: q.Question,
		Rows: rows,
	}
}
