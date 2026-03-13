// internal/opencode/service.go
package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/canhta/gistclaw/internal/channel"
	"github.com/canhta/gistclaw/internal/hitl"
)

// Service is the interface satisfied by *serviceImpl.
// It also satisfies infra.AgentHealthChecker via Name() + IsAlive().
type Service interface {
	Run(ctx context.Context) error
	SubmitTask(ctx context.Context, chatID int64, prompt string) error
	Stop(ctx context.Context) error
	IsAlive(ctx context.Context) bool
	Name() string // returns "opencode"
}

type serviceImpl struct {
	cfg    Config
	ch     channel.Channel
	hitl   hitlApprover
	guard  costTracker
	soul   soulLoader
	client *http.Client

	mu        sync.Mutex
	sessionID string
	cmd       *exec.Cmd
}

// Narrow dependency interfaces — keeps serviceImpl testable without importing infra directly.
// The concrete infra types satisfy these interfaces; tests provide lightweight fakes.

type hitlApprover interface {
	RequestPermission(ctx context.Context, req hitl.PermissionRequest) error
	RequestQuestion(ctx context.Context, req hitl.QuestionRequest) error
}

type costTracker interface {
	Track(usd float64)
}

type soulLoader interface {
	Load() (string, error)
}

// New constructs a new opencode.Service. All dependencies are injected as interfaces.
// In production, pass *infra.CostGuard and *infra.SOULLoader (both satisfy the narrow
// interfaces above). In tests, pass lightweight fakes.
func New(cfg Config, ch channel.Channel, approver hitlApprover, guard costTracker, soul soulLoader) Service {
	return &serviceImpl{
		cfg:    cfg,
		ch:     ch,
		hitl:   approver,
		guard:  guard,
		soul:   soul,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *serviceImpl) Name() string { return "opencode" }

// Run starts opencode serve and blocks until ctx is cancelled or the subprocess exits.
func (s *serviceImpl) Run(ctx context.Context) error {
	args := []string{
		"serve",
		"--port", fmt.Sprintf("%d", s.cfg.Port),
		"--hostname", "127.0.0.1",
		"--dir", s.cfg.Dir,
	}
	cmd := exec.CommandContext(ctx, "opencode", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opencode: start subprocess: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	// Wait up to StartupTimeout seconds (default behaviour: 3s) for health check.
	timeout := s.cfg.StartupTimeout
	if timeout <= 0 {
		timeout = 3
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	for time.Now().Before(deadline) {
		hctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		alive := s.isAliveURL(hctx)
		cancel()
		if alive {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !s.IsAlive(ctx) {
		_ = cmd.Process.Kill()
		return fmt.Errorf("opencode: failed to start within %ds", timeout)
	}

	return cmd.Wait()
}

// SubmitTask submits a prompt to the running OpenCode instance.
func (s *serviceImpl) SubmitTask(ctx context.Context, chatID int64, prompt string) error {
	// Ensure (or reuse) a session.
	sessionID, err := s.ensureSession(ctx)
	if err != nil {
		return fmt.Errorf("opencode: ensure session: %w", err)
	}

	// Load SOUL.md.
	soul, err := s.soul.Load()
	if err != nil {
		log.Warn().Err(err).Msg("opencode: could not load SOUL.md; proceeding without system prompt")
		soul = ""
	}

	// Build and send prompt_async request.
	body, _ := json.Marshal(map[string]interface{}{
		"parts":  []map[string]string{{"type": "text", "text": prompt}},
		"system": soul,
	})
	url := s.base() + "/session/" + sessionID + "/prompt_async"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("opencode: prompt_async: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusInternalServerError {
		bodyBytes, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(bodyBytes), "is busy") {
			_ = s.ch.SendMessage(ctx, chatID, "⚠️ OpenCode is busy. Wait for current task to finish.")
			return nil
		}
		return fmt.Errorf("opencode: prompt_async returned HTTP 500: %s", bodyBytes)
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("opencode: prompt_async returned HTTP %d: %s", resp.StatusCode, bodyBytes)
	}

	// Consume SSE stream.
	return s.consumeSSE(ctx, chatID, sessionID)
}

// Stop aborts the active session and kills the subprocess.
func (s *serviceImpl) Stop(ctx context.Context) error {
	s.mu.Lock()
	sessionID := s.sessionID
	cmd := s.cmd
	s.mu.Unlock()

	if sessionID != "" {
		abortURL := s.base() + "/session/" + sessionID + "/abort"
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, abortURL, nil)
		resp, err := s.client.Do(req)
		if err != nil {
			log.Warn().Err(err).Msg("opencode: abort session")
		} else {
			resp.Body.Close()
		}
		s.mu.Lock()
		s.sessionID = ""
		s.mu.Unlock()
	}

	if cmd != nil && cmd.Process != nil {
		if err := cmd.Process.Kill(); err != nil {
			log.Warn().Err(err).Msg("opencode: kill subprocess")
		}
	}
	return nil
}

// IsAlive checks whether the opencode serve health endpoint responds with 200.
func (s *serviceImpl) IsAlive(ctx context.Context) bool {
	return s.isAliveURL(ctx)
}

// --- private helpers ---

func (s *serviceImpl) base() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.cfg.Port)
}

func (s *serviceImpl) isAliveURL(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base()+"/global/health", nil)
	if err != nil {
		return false
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (s *serviceImpl) ensureSession(ctx context.Context) (string, error) {
	s.mu.Lock()
	existing := s.sessionID
	s.mu.Unlock()
	if existing != "" {
		return existing, nil
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.base()+"/session", nil)
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode session response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("opencode: session response missing id")
	}

	s.mu.Lock()
	s.sessionID = result.ID
	s.mu.Unlock()
	return result.ID, nil
}

func (s *serviceImpl) consumeSSE(ctx context.Context, chatID int64, sessionID string) error {
	url := fmt.Sprintf("%s/event?directory=%s", s.base(), s.cfg.Dir)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Accept", "text/event-stream")

	// Use a client without timeout for the long-running SSE stream.
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("opencode: SSE connect: %w", err)
	}
	defer resp.Body.Close()

	var buf strings.Builder // accumulates text output
	var hadOutput bool      // true once any text part has been received
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		ev, err := ParseSSELine(line)
		if err != nil {
			log.Warn().Err(err).Msg("opencode: skip malformed SSE line")
			continue
		}
		if ev == nil {
			continue
		}

		switch ev.Type {
		case "message.part.updated":
			if ev.Part == nil {
				continue
			}
			switch ev.Part.Type {
			case "text":
				hadOutput = true
				buf.WriteString(ev.Part.Text)
				// Flush to Telegram at 4096-char boundary.
				for buf.Len() >= 4096 {
					chunk := buf.String()[:4096]
					_ = s.ch.SendMessage(ctx, chatID, chunk)
					rest := buf.String()[4096:]
					buf.Reset()
					buf.WriteString(rest)
				}
			case "step-finish":
				s.guard.Track(ev.Part.CostUSD)
			}

		case "permission.asked":
			decisionCh := make(chan hitl.HITLDecision, 1)
			_ = s.hitl.RequestPermission(ctx, hitl.PermissionRequest{
				ChatID:     chatID,
				ID:         ev.ID,
				SessionID:  ev.SessionID,
				Permission: ev.Permission,
				Patterns:   ev.Patterns,
				DecisionCh: decisionCh,
			})

		case "question.asked":
			var questions []hitl.Question
			for _, q := range ev.Questions {
				var opts []hitl.Option
				for _, o := range q.Options {
					opts = append(opts, hitl.Option{Label: o.Label, Description: o.Description})
				}
				questions = append(questions, hitl.Question{
					Question: q.Question,
					Header:   q.Header,
					Options:  opts,
					Multiple: q.Multiple,
					Custom:   q.Custom,
				})
			}
			_ = s.hitl.RequestQuestion(ctx, hitl.QuestionRequest{
				ChatID:    chatID,
				ID:        ev.ID,
				SessionID: ev.SessionID,
				Questions: questions,
			})

		case "session.status":
			if ev.Status != nil && ev.Status.Type == "idle" {
				// Clear session.
				s.mu.Lock()
				s.sessionID = ""
				s.mu.Unlock()
				// Send completion or zero-output message.
				if !hadOutput {
					_ = s.ch.SendMessage(ctx, chatID, "⚠️ Agent finished but produced no output.")
				} else {
					// Flush any remaining buffered text.
					if buf.Len() > 0 {
						_ = s.ch.SendMessage(ctx, chatID, buf.String())
						buf.Reset()
					}
					_ = s.ch.SendMessage(ctx, chatID, "✅ Done")
				}
				return nil
			}
		}
	}
	return scanner.Err()
}
