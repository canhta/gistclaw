package web

import (
	"context"
	"sync"

	"github.com/canhta/gistclaw/internal/model"
)

type SSEBroadcaster struct {
	mu          sync.RWMutex
	subscribers map[string][]chan model.ReplayDelta
}

func NewSSEBroadcaster() *SSEBroadcaster {
	return &SSEBroadcaster{
		subscribers: make(map[string][]chan model.ReplayDelta),
	}
}

func (b *SSEBroadcaster) Subscribe(runID string) chan model.ReplayDelta {
	ch := make(chan model.ReplayDelta, 8)

	b.mu.Lock()
	b.subscribers[runID] = append(b.subscribers[runID], ch)
	b.mu.Unlock()

	return ch
}

func (b *SSEBroadcaster) Unsubscribe(runID string, target chan model.ReplayDelta) {
	b.mu.Lock()
	defer b.mu.Unlock()

	current := b.subscribers[runID]
	if len(current) == 0 {
		return
	}

	next := current[:0]
	for _, ch := range current {
		if ch != target {
			next = append(next, ch)
		}
	}

	if len(next) == 0 {
		delete(b.subscribers, runID)
		return
	}

	b.subscribers[runID] = next
}

func (b *SSEBroadcaster) Emit(_ context.Context, runID string, evt model.ReplayDelta) error {
	b.mu.RLock()
	subscribers := append([]chan model.ReplayDelta(nil), b.subscribers[runID]...)
	b.mu.RUnlock()

	for _, ch := range subscribers {
		select {
		case ch <- evt:
		default:
		}
	}

	return nil
}

var _ model.RunEventSink = (*SSEBroadcaster)(nil)
