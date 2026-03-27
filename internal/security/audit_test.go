package security

import (
	"strings"
	"testing"

	"github.com/canhta/gistclaw/internal/app"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/tools"
)

func TestRunAudit(t *testing.T) {
	workspaceRoot := t.TempDir()

	baseConfig := app.Config{
		StorageRoot: workspaceRoot,
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
		wantDetailParts   []string
		wantFixParts      []string
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
				StorageRoot: workspaceRoot,
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
			wantDetailParts: []string{
				"0.0.0.0:8080",
			},
			wantFixParts: []string{
				"127.0.0.1",
				"reverse proxy",
				"gistclaw auth set-password",
			},
		},
		{
			name: "invalid research provider",
			cfg: app.Config{
				StorageRoot: workspaceRoot,
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
				StorageRoot: workspaceRoot,
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
				StorageRoot: workspaceRoot,
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
			for _, part := range tt.wantDetailParts {
				if !strings.Contains(finding.Detail, part) {
					t.Fatalf("expected detail %q to contain %q", finding.Detail, part)
				}
			}
			for _, part := range tt.wantFixParts {
				if !strings.Contains(finding.Remediation, part) {
					t.Fatalf("expected remediation %q to contain %q", finding.Remediation, part)
				}
			}
		})
	}
}

func TestAuditWarnsWhenZaloPersonalEnabled(t *testing.T) {
	workspaceRoot := t.TempDir()

	report := RunAudit(Input{
		Config: app.Config{
			StorageRoot: workspaceRoot,
			Provider: app.ProviderConfig{
				Name:   "anthropic",
				APIKey: "sk-test",
			},
			ZaloPersonal: app.ZaloPersonalConfig{
				Enabled: true,
			},
			Web: app.WebConfig{
				ListenAddr: "127.0.0.1:8080",
			},
		},
		AdminTokenPresent: true,
	})

	finding, ok := findingByID(report.Findings, "zalo_personal.unofficial")
	if !ok {
		t.Fatalf("expected unofficial zalo personal warning, got %#v", report.Findings)
	}
	if finding.Severity != SeverityWarn {
		t.Fatalf("expected warn severity, got %q", finding.Severity)
	}
	for _, part := range []string{"reverse-engineered", "personal-account", "Zalo"} {
		if !strings.Contains(finding.Detail, part) {
			t.Fatalf("expected detail %q to contain %q", finding.Detail, part)
		}
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
