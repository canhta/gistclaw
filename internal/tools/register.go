package tools

import (
	"context"
	"fmt"
	"io"
	"time"
)

type BuildOptions struct {
	Research   ResearchConfig
	MCP        MCPOptions
	MCPFactory MCPFactory
}

func BuildRegistry(ctx context.Context, opts BuildOptions) (*Registry, io.Closer, error) {
	reg := NewRegistry()
	var closers multiCloser

	reg.Register(NewWebFetchTool(newBoundedHTTPClient(researchTimeout(opts.Research)), 1<<20))

	research := normalizeResearchConfig(opts.Research)
	if research.Provider != "" {
		switch research.Provider {
		case "tavily":
			if research.APIKey == "" {
				return nil, nil, fmt.Errorf("tools: research api_key is required for provider %q", research.Provider)
			}
			reg.Register(NewWebSearchTool(newTavilySearchBackend(research), research))
		default:
			return nil, nil, fmt.Errorf("tools: unknown research provider %q", research.Provider)
		}
	}

	mcpCloser, err := loadMCPTools(ctx, reg, opts.MCP, opts.MCPFactory)
	if err != nil {
		return nil, nil, err
	}
	if mcpCloser != nil {
		closers = append(closers, mcpCloser)
	}
	if len(closers) == 0 {
		return reg, nil, nil
	}
	return reg, closers, nil
}

func researchTimeout(cfg ResearchConfig) time.Duration {
	cfg = normalizeResearchConfig(cfg)
	return time.Duration(cfg.TimeoutSec) * time.Second
}

type multiCloser []io.Closer

func (m multiCloser) Close() error {
	var firstErr error
	for _, closer := range m {
		if closer == nil {
			continue
		}
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
