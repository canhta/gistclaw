// internal/channel/telegram/split.go
package telegram

import "strings"

// TelegramLimit is the safe per-message byte limit used when sending to Telegram.
// The raw API limit is 4096 characters; 32 bytes are reserved as a conservative
// buffer for any parse_mode entity wrapping the library may add.
const TelegramLimit = 4096 - 32

// telegramLimit is the internal alias used by SendMessage.
const telegramLimit = TelegramLimit

// SplitMessage splits text into chunks each ≤ limit bytes, suitable for sending
// as consecutive Telegram messages.
//
// Rules:
//   - Splits prefer line boundaries; hard cuts within a line only when a single
//     line exceeds the limit alone.
//   - Code-block fences (` ``` `) are healed across chunk boundaries: the closing
//     chunk ends with ` ``` ` and the opening chunk of the next part begins with
//     ` ```lang `.
//   - SplitMessage("", limit) returns []string{""}.
//
// chunkLen tracks the exact byte length of strings.Join(chunk, "\n") so that
// chunks are never emitted over limit. Separating newlines (one per inter-line
// join) are accounted explicitly via sep.
func SplitMessage(text string, limit int) []string {
	if limit <= 0 {
		limit = 1
	}

	pending := strings.Split(text, "\n") // always ≥1 element, even for ""
	var (
		chunk     []string
		chunkLen  int // == len(strings.Join(chunk, "\n"))
		inBlock   bool
		blockLang string
		result    []string
	)

	for len(pending) > 0 {
		// Step 1: dequeue next line.
		line := pending[0]
		pending = pending[1:]

		// Step 2: detect fence direction before any state toggle.
		isFence := strings.HasPrefix(line, "```")
		isOpening := isFence && !inBlock
		if isOpening {
			blockLang = strings.TrimPrefix(line, "```")
		}

		// sep is the separator byte that joins this line to the existing chunk.
		// It is 0 when the chunk is empty (first line has no leading \n).
		sep := 0
		if len(chunk) > 0 {
			sep = 1
		}
		need := len(line) + sep

		// effectiveLimit reserves 4 bytes ("\n```") for the healing close fence
		// whenever the cursor is inside a code block, so that the healer never
		// pushes a flushed chunk over the real limit.
		effectiveLimit := limit
		if inBlock {
			effectiveLimit = limit - 4 // len("\n```") == 4
		}

		// Step 3: flush current chunk if this line would not fit.
		// Only flush when the chunk is non-empty to avoid emitting empty chunks.
		if chunkLen+need > effectiveLimit && len(chunk) > 0 {
			// Close the open code block before emitting.
			if inBlock {
				chunk = append(chunk, "```")
			}
			result = append(result, strings.Join(chunk, "\n"))
			chunk = chunk[:0]
			chunkLen = 0

			// First line of new chunk has no separator.
			sep = 0
			need = len(line)

			// Reopen the block in the new chunk.
			if inBlock {
				opener := "```" + blockLang
				chunk = append(chunk, opener)
				chunkLen = len(opener)
				sep = 1
				need = len(line) + sep
			}

			// Recompute effectiveLimit for the remainder of this iteration
			// (chunkLen changed; inBlock is unchanged until step 6).
			effectiveLimit = limit
			if inBlock {
				effectiveLimit = limit - 4
			}
		}

		// Step 4: hard-cut fallback — triggers only when the line still doesn't fit
		// after the flush above (e.g. a line longer than limit itself).
		if chunkLen+need > effectiveLimit {
			available := effectiveLimit - chunkLen - sep
			if available <= 0 {
				available = 1 // always advance to prevent infinite loop
			}
			if available > len(line) {
				available = len(line)
			}
			rest := line[available:]
			line = line[:available]
			need = available + sep
			if rest != "" {
				// Re-queue the remainder at the front of pending.
				pending = append([]string{rest}, pending...)
			}
			// A hard-cut fragment is never a complete fence line.
			isFence = false
		}

		// Step 5: append line to current chunk.
		chunk = append(chunk, line)
		chunkLen += need

		// Step 6: toggle fence state after appending (so flush logic used pre-toggle state).
		if isFence {
			if isOpening {
				inBlock = true
			} else {
				inBlock = false
				blockLang = ""
			}
		}
	}

	// Final flush.
	if len(chunk) > 0 {
		if inBlock {
			chunk = append(chunk, "```")
		}
		result = append(result, strings.Join(chunk, "\n"))
	}

	return result
}
