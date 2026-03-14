package gateway

import "github.com/canhta/gistclaw/internal/providers"

// compressMessages reduces the message history in-place when a context-window
// limit is hit. It first drops the oldest floor(N/2) assistant+tool dyads from
// the candidate window (all non-system turns except the last 4). If there are
// no droppable dyads, it drops the oldest plain conversation messages from that
// candidate window while preserving all system messages and the last 4
// non-system messages.
func compressMessages(msgs *[]providers.Message) bool {
	var system []providers.Message
	var turns []providers.Message
	for _, m := range *msgs {
		if m.Role == "system" {
			system = append(system, m)
			continue
		}
		turns = append(turns, m)
	}

	if len(turns) <= 4 {
		return false
	}

	tailStart := len(turns) - 4
	candidates := turns[:tailStart]
	tail := turns[tailStart:]

	type dyad struct{ start, end int }
	var dyads []dyad
	for i := 0; i < len(candidates)-1; i++ {
		if isDroppableToolDyad(candidates[i], candidates[i+1]) {
			dyads = append(dyads, dyad{start: i, end: i + 1})
			i++
		}
	}

	drop := len(dyads) / 2
	if drop > 0 {
		dropIdx := make(map[int]bool, drop*2)
		for _, d := range dyads[:drop] {
			dropIdx[d.start] = true
			dropIdx[d.end] = true
		}

		result := make([]providers.Message, 0, len(*msgs)-drop*2)
		result = append(result, system...)
		for i, m := range candidates {
			if !dropIdx[i] {
				result = append(result, m)
			}
		}
		result = append(result, tail...)
		*msgs = result
		return true
	}

	var plainCandidateIdx []int
	for i, m := range candidates {
		if m.Role == "user" || (m.Role == "assistant" && m.ToolCallID == "" && m.ToolName == "") {
			plainCandidateIdx = append(plainCandidateIdx, i)
		}
	}

	if len(plainCandidateIdx) == 0 {
		return false
	}

	plainDrop := len(plainCandidateIdx) / 2
	if plainDrop == 0 {
		plainDrop = 1
	}

	dropIdx := make(map[int]bool, plainDrop)
	for _, idx := range plainCandidateIdx[:plainDrop] {
		dropIdx[idx] = true
	}

	result := make([]providers.Message, 0, len(*msgs)-plainDrop)
	result = append(result, system...)
	for i, m := range candidates {
		if !dropIdx[i] {
			result = append(result, m)
		}
	}
	result = append(result, tail...)
	*msgs = result
	return true
}

func isDroppableToolDyad(assistant, tool providers.Message) bool {
	return assistant.Role == "assistant" &&
		assistant.ToolCallID != "" &&
		assistant.ToolName != "" &&
		tool.Role == "tool" &&
		tool.ToolCallID != "" &&
		tool.ToolCallID == assistant.ToolCallID
}
