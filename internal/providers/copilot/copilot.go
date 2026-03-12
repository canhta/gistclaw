// internal/providers/copilot/copilot.go
package copilot

import (
	"context"
	"encoding/json"
	"fmt"

	copilot "github.com/github/copilot-sdk/go"

	"github.com/canhta/gistclaw/internal/providers"
)

// Provider implements providers.LLMProvider via the GitHub Copilot SDK.
//
// The Copilot bridge runs at grpcAddr (e.g. "localhost:4321").
// TotalCostUSD is always 0 — billing is opaque on the bridge.
//
// Each Chat call creates a temporary session, sends the marshaled message
// history as a single JSON prompt, waits for the assistant reply, then
// destroys the session. This keeps the implementation stateless.
type Provider struct {
	grpcAddr string
}

// New creates a Copilot provider targeting the given bridge address.
func New(grpcAddr string) *Provider {
	return &Provider{grpcAddr: grpcAddr}
}

// Name implements providers.LLMProvider.
func (p *Provider) Name() string { return "copilot" }

// chatMsg is the on-wire JSON shape sent to the Copilot bridge.
type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Chat implements providers.LLMProvider.
//
// It connects to the Copilot bridge at p.grpcAddr, creates a single-use
// session, encodes the message history as a JSON array, sends it, waits for
// the assistant reply, destroys the session, and returns the response.
// TotalCostUSD is always 0 because Copilot billing is opaque.
func (p *Provider) Chat(ctx context.Context, msgs []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	// Encode message history.
	wire := make([]chatMsg, 0, len(msgs))
	for _, m := range msgs {
		wire = append(wire, chatMsg{Role: m.Role, Content: m.Content})
	}
	prompt, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("copilot: marshal messages: %w", err)
	}

	// Connect to the external bridge (does not spawn a subprocess).
	client := copilot.NewClient(&copilot.ClientOptions{
		CLIUrl: p.grpcAddr,
	})
	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("copilot: connect to bridge %s: %w", p.grpcAddr, err)
	}
	defer client.Stop() //nolint:errcheck

	// Create a single-use session.
	session, err := client.CreateSession(ctx, &copilot.SessionConfig{
		OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
	})
	if err != nil {
		return nil, fmt.Errorf("copilot: create session: %w", err)
	}
	defer session.Destroy() //nolint:errcheck

	// Send the prompt and wait for the assistant reply.
	resp, err := session.SendAndWait(ctx, copilot.MessageOptions{
		Prompt: string(prompt),
	})
	if err != nil {
		return nil, fmt.Errorf("copilot: send and wait: %w", err)
	}
	if resp == nil || resp.Data.Content == nil {
		return nil, fmt.Errorf("copilot: empty response from bridge")
	}

	return &providers.LLMResponse{
		Content: *resp.Data.Content,
		Usage:   providers.Usage{TotalCostUSD: 0},
	}, nil
}
