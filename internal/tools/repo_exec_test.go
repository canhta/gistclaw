package tools

import (
	"context"
	"encoding/json"
	"slices"
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
