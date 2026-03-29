package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canhta/gistclaw/internal/extensionstatus"
)

type stubExtensionStatusSource struct {
	status extensionstatus.Status
	err    error
}

func (s stubExtensionStatusSource) ExtensionStatus(context.Context) (extensionstatus.Status, error) {
	if s.err != nil {
		return extensionstatus.Status{}, s.err
	}
	return s.status, nil
}

func TestSkillsStatusReturnsExtensionSnapshot(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)
	h.rawServer.extensions = stubExtensionStatusSource{
		status: extensionstatus.Status{
			Summary: extensionstatus.Summary{
				ShippedSurfaces:    6,
				ConfiguredSurfaces: 4,
				InstalledTools:     18,
				ReadyCredentials:   4,
				MissingCredentials: 1,
			},
			Surfaces: []extensionstatus.Surface{
				{
					ID:                   "anthropic",
					Name:                 "Anthropic",
					Kind:                 "provider",
					Configured:           true,
					Active:               true,
					CredentialState:      "ready",
					CredentialStateLabel: "ready",
					Summary:              "Primary provider is configured.",
					Detail:               "cheap claude-3-haiku · strong claude-sonnet",
				},
				{
					ID:                   "telegram",
					Name:                 "Telegram",
					Kind:                 "connector",
					Configured:           true,
					Active:               true,
					CredentialState:      "ready",
					CredentialStateLabel: "ready",
					Summary:              "Bot token configured.",
					Detail:               "Front agent assistant",
				},
			},
			Tools: []extensionstatus.Tool{
				{
					Name:        "connector_send",
					Family:      "connector",
					Risk:        "medium",
					Approval:    "required",
					SideEffect:  "connector_send",
					Description: "Send a direct message through a registered connector target.",
				},
			},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Summary struct {
			ShippedSurfaces    int `json:"shipped_surfaces"`
			ConfiguredSurfaces int `json:"configured_surfaces"`
			InstalledTools     int `json:"installed_tools"`
			ReadyCredentials   int `json:"ready_credentials"`
		} `json:"summary"`
		Surfaces []struct {
			ID              string `json:"id"`
			Kind            string `json:"kind"`
			CredentialState string `json:"credential_state"`
		} `json:"surfaces"`
		Tools []struct {
			Name     string `json:"name"`
			Approval string `json:"approval"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode skills response: %v", err)
	}

	if resp.Summary.ShippedSurfaces != 6 || resp.Summary.ConfiguredSurfaces != 4 || resp.Summary.InstalledTools != 18 || resp.Summary.ReadyCredentials != 4 {
		t.Fatalf("unexpected summary: %+v", resp.Summary)
	}
	if len(resp.Surfaces) != 2 || resp.Surfaces[0].ID != "anthropic" || resp.Surfaces[1].CredentialState != "ready" {
		t.Fatalf("unexpected surfaces: %+v", resp.Surfaces)
	}
	if len(resp.Tools) != 1 || resp.Tools[0].Name != "connector_send" || resp.Tools[0].Approval != "required" {
		t.Fatalf("unexpected tools: %+v", resp.Tools)
	}
}

func TestSkillsStatusReturnsFallbackWhenSourceMissing(t *testing.T) {
	t.Parallel()

	h := newServerHarness(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	h.server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Notice  string `json:"notice"`
		Summary struct {
			ShippedSurfaces int `json:"shipped_surfaces"`
			InstalledTools  int `json:"installed_tools"`
		} `json:"summary"`
		Surfaces []struct {
			ID string `json:"id"`
		} `json:"surfaces"`
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode skills fallback response: %v", err)
	}

	if resp.Notice != "Extension status source is not wired into this daemon." {
		t.Fatalf("unexpected notice: %q", resp.Notice)
	}
	if resp.Summary.ShippedSurfaces != 0 || resp.Summary.InstalledTools != 0 {
		t.Fatalf("unexpected fallback summary: %+v", resp.Summary)
	}
	if len(resp.Surfaces) != 0 || len(resp.Tools) != 0 {
		t.Fatalf("unexpected fallback payload: surfaces=%+v tools=%+v", resp.Surfaces, resp.Tools)
	}
}
