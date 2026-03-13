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

// New constructs an LLMProvider from cfg.
// If cfg.LLMProviders has multiple entries, returns a ProviderRouter with ordered fallback.
// If only one provider is configured (legacy LLM_PROVIDER or single-entry LLM_PROVIDERS),
// returns it directly — no router overhead for the common single-provider case.
func New(cfg config.Config, s *store.Store) (providers.LLMProvider, error) {
	providerNames := cfg.EffectiveLLMProviders()
	impls := make([]providers.LLMProvider, 0, len(providerNames))
	for _, name := range providerNames {
		p, err := buildOne(name, cfg, s)
		if err != nil {
			return nil, err
		}
		impls = append(impls, p)
	}
	if len(impls) == 1 {
		return impls[0], nil
	}
	return providers.NewProviderRouter(impls, cfg.LLMCooldownWindow), nil
}

func buildOne(name string, cfg config.Config, s *store.Store) (providers.LLMProvider, error) {
	switch name {
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
		return nil, fmt.Errorf("factory: unknown provider %q (valid: openai-key, copilot, codex-oauth)", name)
	}
}
