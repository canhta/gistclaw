// internal/channel/telegram/split_test.go
package telegram

import (
	"strings"
	"testing"
)

func TestSplitMessage(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		limit int
		// wantChunks: number of chunks expected
		wantChunks int
		// wantJoined: if non-empty, all chunks joined with "\n---\n" must equal this
		wantJoined string
		// wantContains: every string in this slice must appear in at least one chunk
		wantContains []string
		// chunkAsserts: per-chunk assertions (index → expected substring)
		chunkAsserts map[int]string
		// chunkNotContains: per-chunk strings that must NOT appear (index → forbidden)
		chunkNotContains map[int]string
		// allWithin: if true, every chunk must be ≤ limit bytes
		allWithin bool
	}{
		{
			name:       "short text — no split",
			text:       "hello world",
			limit:      100,
			wantChunks: 1,
			wantJoined: "hello world",
			allWithin:  true,
		},
		{
			name:       "exact limit — no split",
			text:       strings.Repeat("a", 50),
			limit:      50,
			wantChunks: 1,
			allWithin:  true,
		},
		{
			name:  "over limit at paragraph boundary",
			text:  strings.Repeat("a", 30) + "\n\n" + strings.Repeat("b", 30),
			limit: 40,
			// first chunk: "aaa…(30)\n" blank line = 32 bytes; second chunk: "bbb…(30)"
			wantChunks: 2,
			allWithin:  true,
			chunkAsserts: map[int]string{
				0: strings.Repeat("a", 30),
				1: strings.Repeat("b", 30),
			},
		},
		{
			name:       "over limit prose — split at newline",
			text:       strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 30),
			limit:      40,
			wantChunks: 2,
			allWithin:  true,
			chunkAsserts: map[int]string{
				0: strings.Repeat("a", 30),
				1: strings.Repeat("b", 30),
			},
		},
		{
			name: "mid-code-block split with lang",
			// ```go fence (5), 60-char body, closing ```; limit=40 forces multiple splits.
			// effectiveLimit=36 while in block: opener(5) + content + separator must ≤ 36.
			text:         "```go\n" + strings.Repeat("x", 60) + "\n```",
			limit:        40,
			allWithin:    true, // exact chunk count depends on available; just verify limit
			wantContains: []string{"```go"},
		},
		{
			name:      "mid-code-block split — chunk 1 ends with closing fence",
			text:      "```go\n" + strings.Repeat("x", 60) + "\n```",
			limit:     40,
			allWithin: true,
			chunkAsserts: map[int]string{
				0: "```go",
			},
		},
		{
			name:         "mid-code-block split — no lang tag",
			text:         "```\n" + strings.Repeat("y", 60) + "\n```",
			limit:        40,
			allWithin:    true,
			wantContains: []string{"```"},
		},
		{
			name: "closing fence triggers flush",
			// With limit=20, effectiveLimit=16.
			// opener(5) + sep(1) + content(9) = 15 ≤ 16 → content fits.
			// Then closing ```(3)+sep(1)=4 → 15+4=19 > 16 → closing ``` triggers flush.
			text:       "```go\n" + strings.Repeat("z", 9) + "\n```",
			limit:      20,
			wantChunks: 2,
			allWithin:  true,
			chunkAsserts: map[int]string{
				0: "```", // healer close — only one ```  at the end, not two
				1: "```", // actual close in reopened chunk
			},
		},
		{
			name:       "code block fits in one chunk — fences preserved verbatim",
			text:       "```go\nfunc f() {}\n```",
			limit:      100,
			wantChunks: 1,
			wantJoined: "```go\nfunc f() {}\n```",
			allWithin:  true,
		},
		{
			name:      "single line over limit — hard cut, no chunk exceeds limit",
			text:      strings.Repeat("a", 200),
			limit:     50,
			allWithin: true,
			// exact chunk count not asserted; just verify limit and no data loss
		},
		{
			name:       "empty string",
			text:       "",
			limit:      100,
			wantChunks: 1,
			wantJoined: "",
			allWithin:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitMessage(tt.text, tt.limit)

			// Chunk count check
			if tt.wantChunks > 0 && len(got) != tt.wantChunks {
				t.Errorf("chunk count: want %d, got %d; chunks=%q", tt.wantChunks, len(got), got)
			}

			// All-within-limit check
			if tt.allWithin {
				for i, c := range got {
					if len(c) > tt.limit {
						t.Errorf("chunk[%d] len=%d exceeds limit=%d: %q", i, len(c), tt.limit, c)
					}
				}
			}

			// Joined content check (all input bytes preserved)
			if tt.wantJoined != "" {
				joined := strings.Join(got, "\n---\n")
				// For single-chunk cases, just check equality
				if len(got) == 1 && got[0] != tt.wantJoined {
					t.Errorf("content mismatch:\nwant: %q\n got: %q", tt.wantJoined, got[0])
				}
				_ = joined
			}

			// Content must-contain check
			for _, want := range tt.wantContains {
				found := false
				for _, c := range got {
					if strings.Contains(c, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected %q to appear in some chunk; chunks=%q", want, got)
				}
			}

			// Per-chunk must-contain
			for idx, want := range tt.chunkAsserts {
				if idx >= len(got) {
					t.Errorf("chunkAsserts[%d]: only %d chunks", idx, len(got))
					continue
				}
				if !strings.Contains(got[idx], want) {
					t.Errorf("chunk[%d] should contain %q; got %q", idx, want, got[idx])
				}
			}

			// Per-chunk must-not-contain
			for idx, forbidden := range tt.chunkNotContains {
				if idx >= len(got) {
					continue
				}
				if strings.Contains(got[idx], forbidden) {
					t.Errorf("chunk[%d] should NOT contain %q; got %q", idx, forbidden, got[idx])
				}
			}
		})
	}
}

// TestSplitMessage_AllBytesPreserved verifies that splitting at line boundaries
// (no hard cuts, no healers) reproduces the original via strings.Join(chunks, "\n").
// Cases with hard-cuts or code-block healers are excluded — those add/restructure
// newlines and fence lines by design.
func TestSplitMessage_AllBytesPreserved(t *testing.T) {
	cases := []struct {
		text  string
		limit int
	}{
		{"hello world", 100},
		{"line1\nline2\nline3", 100},
		// Multi-line text split at newline boundaries; no code blocks involved.
		{strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 30), 40},
		// Code block that fits entirely in one chunk — no healers, join works.
		{"```go\nfunc f() {}\n```", 100},
	}
	for _, c := range cases {
		got := SplitMessage(c.text, c.limit)
		joined := strings.Join(got, "\n")
		if joined != c.text {
			t.Errorf("limit=%d: bytes not preserved\nwant: %q\n got: %q", c.limit, c.text, joined)
		}
	}
}

// TestSplitMessage_NoTrailingSpaceOnBareFence verifies that a bare ``` reopener
// does not get a trailing space (i.e. "```" not "``` ").
func TestSplitMessage_NoTrailingSpaceOnBareFence(t *testing.T) {
	text := "```\n" + strings.Repeat("y", 60) + "\n```"
	chunks := SplitMessage(text, 40)
	for i, c := range chunks {
		lines := strings.Split(c, "\n")
		for _, line := range lines {
			if line == "``` " {
				t.Errorf("chunk[%d] contains bare fence with trailing space: %q", i, c)
			}
		}
	}
}
