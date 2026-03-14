package gateway

import (
	"fmt"
	"testing"

	"github.com/canhta/gistclaw/internal/providers"
)

func msg(role, content string) providers.Message {
	return providers.Message{Role: role, Content: content}
}

func toolCallMsg(id, name, content string) providers.Message {
	return providers.Message{Role: "assistant", Content: content, ToolCallID: id, ToolName: name}
}

func toolResultMsg(id, content string) providers.Message {
	return providers.Message{Role: "tool", Content: content, ToolCallID: id}
}

func cloneMessages(msgs []providers.Message) []providers.Message {
	clone := make([]providers.Message, len(msgs))
	copy(clone, msgs)
	return clone
}

func assertMessagesEqual(t *testing.T, got, want []providers.Message) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d; want %d\ngot:  %s\nwant: %s", len(got), len(want), formatMessages(got), formatMessages(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message[%d] mismatch: got %+v; want %+v\ngot:  %s\nwant: %s", i, got[i], want[i], formatMessages(got), formatMessages(want))
		}
	}
}

func formatMessages(msgs []providers.Message) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		parts = append(parts, fmt.Sprintf("%s:%s", m.Role, m.Content))
	}
	return fmt.Sprintf("%v", parts)
}

// TestCompressMessages_NoOp_FewTurns: <=4 non-system messages -> no-op.
func TestCompressMessages_NoOp_FewTurns(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		msg("user", "u1"),
		msg("assistant", "a1"),
		msg("tool", "t1"),
		msg("user", "u2"),
	}
	original := cloneMessages(msgs)
	if compressMessages(&msgs) {
		t.Error("expected no-op (false) for <=4 turns")
	}
	assertMessagesEqual(t, msgs, original)
}

// TestCompressMessages_FallbackDropsOldestPlainTurns: when dyad pruning cannot help,
// drop the oldest plain conversation turns from the candidate window.
func TestCompressMessages_FallbackDropsOldestPlainTurns(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		msg("user", "drop-u1"),
		msg("assistant", "drop-a1"),
		msg("user", "keep-u2"),
		msg("assistant", "keep-a2"),
		msg("user", "tail1"),
		msg("assistant", "tail2"),
		msg("user", "tail3"),
		msg("assistant", "tail4"),
	}
	if !compressMessages(&msgs) {
		t.Fatal("expected fallback compression when no dyads are available")
	}
	assertMessagesEqual(t, msgs, []providers.Message{
		msg("system", "sys"),
		msg("user", "keep-u2"),
		msg("assistant", "keep-a2"),
		msg("user", "tail1"),
		msg("assistant", "tail2"),
		msg("user", "tail3"),
		msg("assistant", "tail4"),
	})
}

func TestCompressMessages_FallbackDropsSingleOldestPlainTurnWhenOnlyOneCandidate(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		msg("user", "drop-u1"),
		msg("assistant", "tail-a1"),
		msg("user", "tail-u2"),
		msg("assistant", "tail-a2"),
		msg("user", "tail-u3"),
	}
	if !compressMessages(&msgs) {
		t.Fatal("expected fallback compression with one plain candidate turn")
	}
	assertMessagesEqual(t, msgs, []providers.Message{
		msg("system", "sys"),
		msg("assistant", "tail-a1"),
		msg("user", "tail-u2"),
		msg("assistant", "tail-a2"),
		msg("user", "tail-u3"),
	})
}

// TestCompressMessages_NoOp_OneDyad: floor(1/2)=0 -> no-op.
func TestCompressMessages_NoOp_OneDyad(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		toolCallMsg("call-0", "web_search", "a0"), toolResultMsg("call-0", "t0"),
		msg("user", "tail1"),
		toolCallMsg("call-1", "web_search", "a1"), toolResultMsg("call-1", "t1"),
		msg("user", "tail2"),
	}
	original := cloneMessages(msgs)
	if compressMessages(&msgs) {
		t.Error("expected no-op (false) for 1 dyad where floor(1/2)=0")
	}
	assertMessagesEqual(t, msgs, original)
}

// TestCompressMessages_DropsDyads: 2 dyads -> drop floor(2/2)=1 oldest.
func TestCompressMessages_DropsDyads(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		toolCallMsg("drop-call", "web_search", "drop-a"), toolResultMsg("drop-call", "drop-t"),
		toolCallMsg("keep-call", "web_search", "keep-a"), toolResultMsg("keep-call", "keep-t"),
		msg("user", "tail-u1"),
		toolCallMsg("tail-call", "web_search", "tail-a"), toolResultMsg("tail-call", "tail-t"),
		msg("user", "tail-u2"),
	}
	if !compressMessages(&msgs) {
		t.Fatal("expected compression (true)")
	}
	assertMessagesEqual(t, msgs, []providers.Message{
		msg("system", "sys"),
		toolCallMsg("keep-call", "web_search", "keep-a"),
		toolResultMsg("keep-call", "keep-t"),
		msg("user", "tail-u1"),
		toolCallMsg("tail-call", "web_search", "tail-a"),
		toolResultMsg("tail-call", "tail-t"),
		msg("user", "tail-u2"),
	})
}

// TestCompressMessages_UserMessagesNeverDropped: user in candidates always survives.
func TestCompressMessages_UserMessagesNeverDropped(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		msg("user", "must-keep"),
		toolCallMsg("call-1", "web_search", "a1"), toolResultMsg("call-1", "t1"),
		toolCallMsg("call-2", "web_search", "a2"), toolResultMsg("call-2", "t2"),
		toolCallMsg("call-3", "web_search", "a3"), toolResultMsg("call-3", "t3"),
		toolCallMsg("call-4", "web_search", "a4"), toolResultMsg("call-4", "t4"),
		msg("user", "tail1"),
		toolCallMsg("tail-call", "web_search", "tail-a"), toolResultMsg("tail-call", "tail-t"),
		msg("user", "tail2"),
	}
	if !compressMessages(&msgs) {
		t.Fatal("expected compression")
	}
	assertMessagesEqual(t, msgs, []providers.Message{
		msg("system", "sys"),
		msg("user", "must-keep"),
		toolCallMsg("call-3", "web_search", "a3"),
		toolResultMsg("call-3", "t3"),
		toolCallMsg("call-4", "web_search", "a4"),
		toolResultMsg("call-4", "t4"),
		msg("user", "tail1"),
		toolCallMsg("tail-call", "web_search", "tail-a"),
		toolResultMsg("tail-call", "tail-t"),
		msg("user", "tail2"),
	})
}

// TestCompressMessages_SystemMessagesAlwaysKept.
func TestCompressMessages_SystemMessagesAlwaysKept(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys1"),
		msg("system", "sys2"),
		toolCallMsg("call-1", "web_search", "a1"), toolResultMsg("call-1", "t1"),
		toolCallMsg("call-2", "web_search", "a2"), toolResultMsg("call-2", "t2"),
		toolCallMsg("call-3", "web_search", "a3"), toolResultMsg("call-3", "t3"),
		toolCallMsg("call-4", "web_search", "a4"), toolResultMsg("call-4", "t4"),
		msg("user", "tail1"),
		toolCallMsg("tail-call", "web_search", "tail-a"), toolResultMsg("tail-call", "tail-t"),
		msg("user", "tail2"),
	}
	if !compressMessages(&msgs) {
		t.Fatal("expected compression")
	}
	assertMessagesEqual(t, msgs, []providers.Message{
		msg("system", "sys1"),
		msg("system", "sys2"),
		toolCallMsg("call-3", "web_search", "a3"),
		toolResultMsg("call-3", "t3"),
		toolCallMsg("call-4", "web_search", "a4"),
		toolResultMsg("call-4", "t4"),
		msg("user", "tail1"),
		toolCallMsg("tail-call", "web_search", "tail-a"),
		toolResultMsg("tail-call", "tail-t"),
		msg("user", "tail2"),
	})
}

func TestCompressMessages_FallbackKeepsStrayToolMessages(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		msg("assistant", "drop-a"),
		msg("user", "keep-u"),
		msg("tool", "candidate-t"),
		msg("user", "tail1"),
		msg("assistant", "tail-a"), msg("tool", "tail-t"),
		msg("user", "tail2"),
	}
	if !compressMessages(&msgs) {
		t.Fatal("expected fallback compression when assistant and tool are not consecutive")
	}
	assertMessagesEqual(t, msgs, []providers.Message{
		msg("system", "sys"),
		msg("user", "keep-u"),
		msg("tool", "candidate-t"),
		msg("user", "tail1"),
		msg("assistant", "tail-a"), msg("tool", "tail-t"),
		msg("user", "tail2"),
	})
}

func TestCompressMessages_StrayToolNotDroppedOnItsOwn(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		msg("tool", "stray-tool"),
		toolCallMsg("call-1", "web_search", "drop-a1"), toolResultMsg("call-1", "drop-t1"),
		toolCallMsg("call-2", "web_search", "drop-a2"), toolResultMsg("call-2", "drop-t2"),
		toolCallMsg("call-3", "web_search", "keep-a3"), toolResultMsg("call-3", "keep-t3"),
		toolCallMsg("call-4", "web_search", "keep-a4"), toolResultMsg("call-4", "keep-t4"),
		msg("user", "tail1"),
		toolCallMsg("tail-call", "web_search", "tail-a"), toolResultMsg("tail-call", "tail-t"),
		msg("user", "tail2"),
	}
	if !compressMessages(&msgs) {
		t.Fatal("expected compression")
	}
	assertMessagesEqual(t, msgs, []providers.Message{
		msg("system", "sys"),
		msg("tool", "stray-tool"),
		toolCallMsg("call-3", "web_search", "keep-a3"),
		toolResultMsg("call-3", "keep-t3"),
		toolCallMsg("call-4", "web_search", "keep-a4"),
		toolResultMsg("call-4", "keep-t4"),
		msg("user", "tail1"),
		toolCallMsg("tail-call", "web_search", "tail-a"),
		toolResultMsg("tail-call", "tail-t"),
		msg("user", "tail2"),
	})
}

func TestCompressMessages_FallbackIgnoresAdjacentPlainAssistantAndStrayTool(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		msg("assistant", "drop-a1"),
		msg("tool", "keep-stray-tool-1"),
		msg("assistant", "drop-a2"),
		msg("tool", "keep-stray-tool-2"),
		msg("user", "keep-u"),
		msg("user", "tail1"),
		msg("assistant", "tail-a"), msg("tool", "tail-t"),
		msg("user", "tail2"),
	}
	if !compressMessages(&msgs) {
		t.Fatal("expected fallback compression when adjacent assistant/tool pairs lack tool-call metadata")
	}
	assertMessagesEqual(t, msgs, []providers.Message{
		msg("system", "sys"),
		msg("tool", "keep-stray-tool-1"),
		msg("assistant", "drop-a2"),
		msg("tool", "keep-stray-tool-2"),
		msg("user", "keep-u"),
		msg("user", "tail1"),
		msg("assistant", "tail-a"), msg("tool", "tail-t"),
		msg("user", "tail2"),
	})
}

func TestCompressMessages_FallbackIgnoresDyadSplitAcrossCandidateTailBoundary(t *testing.T) {
	msgs := []providers.Message{
		msg("system", "sys"),
		msg("assistant", "drop-a"),
		msg("user", "keep-u"),
		msg("tool", "tail-tool"),
		msg("assistant", "tail-a"),
		msg("tool", "tail-t"),
		msg("user", "tail2"),
	}
	if !compressMessages(&msgs) {
		t.Fatal("expected fallback compression when candidate/tail split prevents dyad pruning")
	}
	assertMessagesEqual(t, msgs, []providers.Message{
		msg("system", "sys"),
		msg("user", "keep-u"),
		msg("tool", "tail-tool"),
		msg("assistant", "tail-a"),
		msg("tool", "tail-t"),
		msg("user", "tail2"),
	})
}
