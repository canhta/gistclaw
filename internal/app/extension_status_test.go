package app

import (
	"context"
	"path/filepath"
	"testing"

	zalopersonal "github.com/canhta/gistclaw/internal/connectors/zalopersonal"
	"github.com/canhta/gistclaw/internal/tools"
)

func TestAppExtensionStatusReportsConfiguredSurfacesToolsAndCredentials(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := Config{
		StorageRoot:  filepath.Join(root, "storage"),
		StateDir:     filepath.Join(root, "state"),
		DatabasePath: filepath.Join(root, "state", "gistclaw.db"),
		Provider: ProviderConfig{
			Name:   "anthropic",
			APIKey: "test-provider-key",
			Models: ModelLanes{
				Cheap:  "claude-3-haiku",
				Strong: "claude-sonnet",
			},
		},
		Research: tools.ResearchConfig{
			Provider:   "tavily",
			APIKey:     "test-research-key",
			MaxResults: 5,
			TimeoutSec: 20,
		},
		Telegram: TelegramConfig{
			BotToken: "telegram-token",
			AgentID:  "assistant",
		},
		WhatsApp: WhatsAppConfig{
			PhoneNumberID: "123456",
			AccessToken:   "whatsapp-token",
			VerifyToken:   "verify-token",
			AgentID:       "assistant",
		},
		ZaloPersonal: ZaloPersonalConfig{
			Enabled: true,
			AgentID: "assistant",
		},
	}

	application, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("bootstrap app: %v", err)
	}
	defer func() { _ = application.Stop() }()

	if err := zalopersonal.SaveStoredCredentials(context.Background(), application.db, zalopersonal.StoredCredentials{
		AccountID: "account-1",
		IMEI:      "imei",
		Cookie:    "cookie",
		UserAgent: "Mozilla/5.0",
	}); err != nil {
		t.Fatalf("save zalo credentials: %v", err)
	}

	status, err := application.ExtensionStatus(context.Background())
	if err != nil {
		t.Fatalf("extension status: %v", err)
	}

	if status.Summary.ConfiguredSurfaces < 4 {
		t.Fatalf("expected configured surfaces, got %+v", status.Summary)
	}
	if status.Summary.InstalledTools < 1 {
		t.Fatalf("expected installed tools, got %+v", status.Summary)
	}
	if status.Summary.ReadyCredentials < 4 {
		t.Fatalf("expected ready credentials, got %+v", status.Summary)
	}

	foundAnthropic := false
	foundTelegram := false
	foundOpenAI := false
	foundConnectorSend := false
	foundWebSearch := false

	for _, surface := range status.Surfaces {
		switch surface.ID {
		case "anthropic":
			foundAnthropic = surface.Active && surface.CredentialState == "ready"
		case "telegram":
			foundTelegram = surface.Configured && surface.Active && surface.CredentialState == "ready"
		case "openai":
			foundOpenAI = !surface.Configured
		}
	}
	for _, tool := range status.Tools {
		if tool.Name == "connector_send" {
			foundConnectorSend = true
		}
		if tool.Name == "web_search" {
			foundWebSearch = true
		}
	}

	if !foundAnthropic {
		t.Fatalf("expected anthropic surface in %+v", status.Surfaces)
	}
	if !foundTelegram {
		t.Fatalf("expected telegram surface in %+v", status.Surfaces)
	}
	if !foundOpenAI {
		t.Fatalf("expected openai availability in %+v", status.Surfaces)
	}
	if !foundConnectorSend || !foundWebSearch {
		t.Fatalf("expected connector_send and web_search tools in %+v", status.Tools)
	}
}
