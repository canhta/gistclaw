package presence

import (
	"context"
	"sync"
	"time"
)

type Options struct {
	StartupDelay           time.Duration
	KeepaliveInterval      time.Duration
	MaxDuration            time.Duration
	MaxConsecutiveFailures int
	StartFn                func(context.Context) error
	StopFn                 func(context.Context) error
}

type Controller struct {
	mu                  sync.Mutex
	opts                Options
	started             bool
	stopped             bool
	stopSent            bool
	cancel              context.CancelFunc
	consecutiveFailures int
}

func NewController(opts Options) *Controller {
	if opts.MaxConsecutiveFailures <= 0 {
		opts.MaxConsecutiveFailures = 2
	}
	return &Controller{opts: opts}
}

func (c *Controller) Start(ctx context.Context) {
	c.mu.Lock()
	if c.started || c.stopped {
		c.mu.Unlock()
		return
	}
	c.started = true
	loopCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	opts := c.opts
	c.mu.Unlock()

	go c.run(loopCtx, opts)
}

func (c *Controller) Stop() {
	c.stop(context.Background())
}

func (c *Controller) MarkOutputStarted() {
	c.stop(context.Background())
}

func (c *Controller) MarkPaused() {
	c.stop(context.Background())
}

func (c *Controller) run(ctx context.Context, opts Options) {
	if !sleepOrDone(ctx, opts.StartupDelay) {
		return
	}
	if !c.emit(ctx, opts) {
		return
	}

	if opts.MaxDuration > 0 {
		timer := time.NewTimer(opts.MaxDuration)
		defer timer.Stop()
		if opts.KeepaliveInterval <= 0 {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				c.stop(context.Background())
				return
			}
		}

		ticker := time.NewTicker(opts.KeepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				c.stop(context.Background())
				return
			case <-ticker.C:
				if !c.emit(ctx, opts) {
					return
				}
			}
		}
	}

	if opts.KeepaliveInterval <= 0 {
		<-ctx.Done()
		return
	}
	ticker := time.NewTicker(opts.KeepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !c.emit(ctx, opts) {
				return
			}
		}
	}
}

func (c *Controller) emit(ctx context.Context, opts Options) bool {
	if opts.StartFn == nil {
		return true
	}
	if err := opts.StartFn(ctx); err != nil {
		c.mu.Lock()
		c.consecutiveFailures++
		failures := c.consecutiveFailures
		limit := c.opts.MaxConsecutiveFailures
		c.mu.Unlock()
		if failures >= limit {
			c.stop(context.Background())
			return false
		}
		return true
	}

	c.mu.Lock()
	c.consecutiveFailures = 0
	c.mu.Unlock()
	return true
}

func (c *Controller) stop(ctx context.Context) {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	c.stopped = true
	cancel := c.cancel
	stopFn := c.opts.StopFn
	shouldStop := !c.stopSent && stopFn != nil
	if shouldStop {
		c.stopSent = true
	}
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if shouldStop {
		_ = stopFn(ctx)
	}
}

func sleepOrDone(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
