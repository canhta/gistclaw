package security

import (
	"testing"

	"github.com/canhta/gistclaw/internal/app"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/tools"
)

func TestRunAudit(t *testing.T) {
	workspaceRoot := t.TempDir()

	baseConfig := app.Config{
		WorkspaceRoot: workspaceRoot,
		Provider: app.ProviderConfig{
			Name:   "anthropic",
			APIKey: "sk-test",
		},
		Web: app.WebConfig{
			ListenAddr: "127.0.0.1:8080",
		},
	}

	tests := []struct {
		name              string
		cfg               app.Config
		adminTokenPresent bool
		wantID            string
		wantSeverity      Severity
	}{
		{
			name:              "missing admin token",
			cfg:               baseConfig,
			adminTokenPresent: false,
			wantID:            "admin_token.missing",
			wantSeverity:      SeverityFail,
		},
		{
			name: "exposed web bind",
			cfg: app.Config{
				WorkspaceRoot: workspaceRoot,
				Provider: app.ProviderConfig{
					Name:   "anthropic",
					APIKey: "sk-test",
				},
				Web: app.WebConfig{
					ListenAddr: "0.0.0.0:8080",
				},
			},
			adminTokenPresent: true,
			wantID:            "web.listen_addr.exposed",
			wantSeverity:      SeverityFail,
		},
		{
			name: "invalid research provider",
			cfg: app.Config{
				WorkspaceRoot: workspaceRoot,
				Provider: app.ProviderConfig{
					Name:   "anthropic",
					APIKey: "sk-test",
				},
				Research: tools.ResearchConfig{
					Provider: "duckduckgo",
					APIKey:   "research-test",
				},
				Web: app.WebConfig{
					ListenAddr: "127.0.0.1:8080",
				},
			},
			adminTokenPresent: true,
			wantID:            "research.provider.invalid",
			wantSeverity:      SeverityFail,
		},
		{
			name: "enabled mcp tool missing binary",
			cfg: app.Config{
				WorkspaceRoot: workspaceRoot,
				Provider: app.ProviderConfig{
					Name:   "anthropic",
					APIKey: "sk-test",
				},
				MCP: tools.MCPOptions{
					Servers: []tools.MCPServerConfig{
						{
							ID:        "github",
							Transport: "stdio",
							Command:   []string{"definitely-not-a-real-mcp-binary"},
							Tools: []tools.MCPToolConfig{
								{
									Name:    "search_repositories",
									Alias:   "github_search_repositories",
									Risk:    model.RiskLow,
									Enabled: true,
								},
							},
						},
					},
				},
				Web: app.WebConfig{
					ListenAddr: "127.0.0.1:8080",
				},
			},
			adminTokenPresent: true,
			wantID:            "mcp.binary.missing",
			wantSeverity:      SeverityWarn,
		},
		{
			name: "incomplete whatsapp config",
			cfg: app.Config{
				WorkspaceRoot: workspaceRoot,
				Provider: app.ProviderConfig{
					Name:   "anthropic",
					APIKey: "sk-test",
				},
				WhatsApp: app.WhatsAppConfig{
					PhoneNumberID: "phone-123",
				},
				Web: app.WebConfig{
					ListenAddr: "127.0.0.1:8080",
				},
			},
			adminTokenPresent: true,
			wantID:            "whatsapp.config.incomplete",
			wantSeverity:      SeverityFail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := RunAudit(Input{
				Config:            tt.cfg,
				AdminTokenPresent: tt.adminTokenPresent,
			})

			finding, ok := findingByID(report.Findings, tt.wantID)
			if !ok {
				t.Fatalf("expected finding %q, got %#v", tt.wantID, report.Findings)
			}
			if finding.Severity != tt.wantSeverity {
				t.Fatalf("expected severity %q, got %q", tt.wantSeverity, finding.Severity)
			}
		})
	}
}

func findingByID(findings []Finding, id string) (Finding, bool) {
	for _, finding := range findings {
		if finding.ID == id {
			return finding, true
		}
	}
	return Finding{}, false
}
