package tools

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
)

type recordingToolLogSink struct {
	mu      sync.Mutex
	records []ToolLogRecord
}

func (s *recordingToolLogSink) Record(_ context.Context, record ToolLogRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return nil
}

func (s *recordingToolLogSink) snapshot() []ToolLogRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ToolLogRecord(nil), s.records...)
}

func TestCommandRunner_StreamsToolLogs(t *testing.T) {
	t.Parallel()

	runner := newCommandRunner(5, 64<<10)
	root := t.TempDir()
	sink := &recordingToolLogSink{}
	ctx := WithInvocationContext(context.Background(), InvocationContext{
		WorkspaceRoot: root,
		LogSink:       sink,
	})

	got, err := runner.run(ctx, commandRequest{
		command: "zsh",
		args: []string{
			"-lc",
			"printf 'line one\\nline two\\n'; printf 'warn one\\n' >&2",
		},
		cwd:    root,
		effect: effectExecWrite,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	records := sink.snapshot()
	if len(records) < 3 {
		t.Fatalf("expected at least 3 log records, got %+v", records)
	}
	pairs := make([]string, 0, len(records))
	for _, record := range records {
		pairs = append(pairs, record.Stream+":"+record.Text)
	}
	for _, want := range []string{
		"stdout:line one\n",
		"stdout:line two\n",
		"stderr:warn one\n",
	} {
		if !slices.Contains(pairs, want) {
			t.Fatalf("expected log %q, got %+v", want, pairs)
		}
	}

	var payload struct {
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Stdout != "line one\nline two\n" {
		t.Fatalf("unexpected stdout payload %q", payload.Stdout)
	}
	if payload.Stderr != "warn one\n" {
		t.Fatalf("unexpected stderr payload %q", payload.Stderr)
	}
}

func TestCommandRunner_StreamsPTYTerminalLogs(t *testing.T) {
	t.Parallel()

	runner := newCommandRunner(5, 64<<10)
	root := t.TempDir()
	sink := &recordingToolLogSink{}
	ctx := WithInvocationContext(context.Background(), InvocationContext{
		WorkspaceRoot: root,
		LogSink:       sink,
	})

	got, err := runner.run(ctx, commandRequest{
		command: "zsh",
		args: []string{
			"-lc",
			"printf '\\033[31mred\\033[0m\\n'; printf 'warn\\n' >&2",
		},
		cwd:    root,
		effect: effectExecWrite,
		usePTY: true,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	records := sink.snapshot()
	if len(records) == 0 {
		t.Fatal("expected terminal log records")
	}
	for _, record := range records {
		if record.Stream != "terminal" {
			t.Fatalf("expected PTY logs to use terminal stream, got %+v", records)
		}
	}

	var payload struct {
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}
	if err := json.Unmarshal([]byte(got.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Stderr != "" {
		t.Fatalf("expected PTY stderr to be merged into terminal stdout, got %q", payload.Stderr)
	}
	if !strings.Contains(payload.Stdout, "red") || !strings.Contains(payload.Stdout, "warn") {
		t.Fatalf("expected PTY stdout to include merged terminal transcript, got %q", payload.Stdout)
	}
}
