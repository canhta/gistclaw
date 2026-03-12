// internal/providers/factory/factory.go
package factory

import (
	"fmt"

	"github.com/canhta/gistclaw/internal/config"
	"github.com/canhta/gistclaw/internal/providers"
	codexprovider "github.com/canhta/gistclaw/internal/providers/codex"
	copilotprovider "github.com/canhta/gistclaw/internal/providers/copilot"
	oaiprovider "github.com/canhta/gistclaw/internal/providers/openai"
	"github.com/canhta/gistclaw/internal/store"
)

// New selects and constructs the LLMProvider implementation based on cfg.LLMProvider.
// Values: "openai-key" (default), "copilot", "codex-oauth".
// Returns an error if the provider value is unrecognised or required credentials
// are missing.
//
// This lives in a separate package to avoid an import cycle:
// providers ← providers/{openai,copilot,codex} ← providers (cycle).
// factory sits above all of them and imports each direction only once.
func New(cfg config.Config, s *store.Store) (providers.LLMProvider, error) {
	switch cfg.LLMProvider {
	case "openai-key":
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("factory: LLM_PROVIDER=openai-key requires OPENAI_API_KEY")
		}
		model := cfg.OpenAIModel
		if model == "" {
			model = "gpt-4o"
		}
		return oaiprovider.New(cfg.OpenAIAPIKey, model), nil

	case "copilot":
		addr := cfg.CopilotGRPCAddr
		if addr == "" {
			addr = "localhost:4321"
		}
		return copilotprovider.New(addr), nil

	case "codex-oauth":
		return codexprovider.New(s), nil

	default:
		return nil, fmt.Errorf("factory: unknown LLM_PROVIDER %q (valid: openai-key, copilot, codex-oauth)", cfg.LLMProvider)
	}
}
