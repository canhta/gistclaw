// internal/providers/copilot/copilot.go
package copilot

import (
	"context"
	"errors"

	"github.com/canhta/gistclaw/internal/providers"
)

// Provider implements providers.LLMProvider via the GitHub Copilot gRPC bridge.
//
// TODO(copilot): Replace stub with real gRPC implementation once
// github.com/github/copilot-sdk/go is publicly available.
// The gRPC bridge runs at cfg.CopilotGRPCAddr (default localhost:4321).
// TotalCostUSD is always 0 — billing is opaque on the gRPC bridge.
type Provider struct {
	grpcAddr string
}

// New creates a Copilot provider targeting the given gRPC bridge address.
func New(grpcAddr string) *Provider {
	return &Provider{grpcAddr: grpcAddr}
}

// Name implements providers.LLMProvider.
func (p *Provider) Name() string { return "copilot" }

// Chat implements providers.LLMProvider.
// Currently returns an error because the Copilot gRPC SDK is not yet publicly available.
// When the SDK is available, this method should:
//  1. Dial p.grpcAddr using grpc.Dial (or grpc.NewClient).
//  2. Create a chat completion request using the SDK client.
//  3. Stream or collect the response.
//  4. Return LLMResponse with TotalCostUSD = 0 (billing is opaque).
func (p *Provider) Chat(_ context.Context, _ []providers.Message, _ []providers.Tool) (*providers.LLMResponse, error) {
	// TODO(copilot): implement when github.com/github/copilot-sdk/go is available.
	return nil, errors.New("copilot: gRPC bridge not available (SDK not yet public)")
}
