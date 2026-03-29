package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/canhta/gistclaw/internal/extensionstatus"
	"github.com/canhta/gistclaw/internal/model"
)

func (a *App) ExtensionStatus(ctx context.Context) (extensionstatus.Status, error) {
	if a == nil {
		return extensionstatus.Status{}, fmt.Errorf("extension status: app is required")
	}

	health, err := a.ConnectorHealth(ctx)
	if err != nil {
		return extensionstatus.Status{}, fmt.Errorf("extension status: load connector health: %w", err)
	}

	surfaces, err := a.loadExtensionSurfaces(ctx, health)
	if err != nil {
		return extensionstatus.Status{}, err
	}
	tools := a.loadExtensionTools()

	status := extensionstatus.Status{
		Surfaces: surfaces,
		Tools:    tools,
	}
	status.Summary = summarizeExtensionStatus(status)
	return status, nil
}

func (a *App) loadExtensionSurfaces(ctx context.Context, health []model.ConnectorHealthSnapshot) ([]extensionstatus.Surface, error) {
	healthByID := make(map[string]model.ConnectorHealthSnapshot, len(health))
	for _, snapshot := range health {
		healthByID[snapshot.ConnectorID] = snapshot
	}

	activeConnectors := make(map[string]model.ConnectorMetadata, len(a.connectors))
	for _, connector := range a.connectors {
		meta := model.NormalizeConnectorMetadata(connector.Metadata())
		if meta.ID == "" {
			continue
		}
		activeConnectors[meta.ID] = meta
	}

	zaloConfigured := false
	var err error
	if a.cfg.ZaloPersonal.Enabled {
		_, zaloConfigured, err = a.ZaloPersonalStoredCredentials(ctx)
		if err != nil {
			return nil, fmt.Errorf("extension status: load zalo personal credentials: %w", err)
		}
	}

	surfaces := []extensionstatus.Surface{
		extensionProviderSurface(a.cfg.Provider, "anthropic"),
		extensionProviderSurface(a.cfg.Provider, "openai"),
		extensionResearchSurface(a.cfg),
	}
	surfaces = append(surfaces, extensionMCPSurfaces(a.cfg)...)
	surfaces = append(surfaces,
		extensionConnectorSurface(
			"telegram",
			"Telegram",
			strings.TrimSpace(a.cfg.Telegram.BotToken) != "",
			strings.TrimSpace(a.cfg.Telegram.BotToken) != "",
			strings.TrimSpace(a.cfg.Telegram.AgentID),
			healthByID["telegram"],
			activeConnectors["telegram"],
		),
		extensionConnectorSurface(
			"whatsapp",
			"WhatsApp",
			strings.TrimSpace(a.cfg.WhatsApp.PhoneNumberID) != "" || strings.TrimSpace(a.cfg.WhatsApp.AccessToken) != "" || strings.TrimSpace(a.cfg.WhatsApp.VerifyToken) != "",
			strings.TrimSpace(a.cfg.WhatsApp.PhoneNumberID) != "" && strings.TrimSpace(a.cfg.WhatsApp.AccessToken) != "" && strings.TrimSpace(a.cfg.WhatsApp.VerifyToken) != "",
			strings.TrimSpace(a.cfg.WhatsApp.AgentID),
			healthByID["whatsapp"],
			activeConnectors["whatsapp"],
		),
		extensionZaloSurface(
			a.cfg,
			zaloConfigured,
			healthByID["zalo_personal"],
			activeConnectors["zalo_personal"],
		),
	)
	return surfaces, nil
}

func extensionProviderSurface(cfg ProviderConfig, providerID string) extensionstatus.Surface {
	active := strings.EqualFold(strings.TrimSpace(cfg.Name), providerID)
	ready := active && strings.TrimSpace(cfg.APIKey) != ""
	summary := "Available in this build."
	detail := "Switch provider.name to " + providerID + " to use compatible endpoints."
	if active {
		summary = "Primary provider is configured."
		models := make([]string, 0, 2)
		if strings.TrimSpace(cfg.Models.Cheap) != "" {
			models = append(models, "cheap "+strings.TrimSpace(cfg.Models.Cheap))
		}
		if strings.TrimSpace(cfg.Models.Strong) != "" {
			models = append(models, "strong "+strings.TrimSpace(cfg.Models.Strong))
		}
		if len(models) > 0 {
			detail = strings.Join(models, " · ")
		} else if strings.TrimSpace(cfg.BaseURL) != "" {
			detail = cfg.BaseURL
		} else {
			detail = "Configured through machine settings and config."
		}
	}

	return extensionstatus.Surface{
		ID:                   providerID,
		Name:                 extensionProviderName(providerID),
		Kind:                 "provider",
		Configured:           active,
		Active:               active,
		CredentialState:      extensionCredentialState(ready, active),
		CredentialStateLabel: extensionCredentialStateLabel(extensionCredentialState(ready, active)),
		Summary:              summary,
		Detail:               detail,
	}
}

func extensionProviderName(providerID string) string {
	if providerID == "openai" {
		return "OpenAI-compatible"
	}
	return "Anthropic"
}

func extensionResearchSurface(cfg Config) extensionstatus.Surface {
	active := strings.EqualFold(strings.TrimSpace(cfg.Research.Provider), "tavily")
	ready := active && strings.TrimSpace(cfg.Research.APIKey) != ""
	summary := "Research is optional in this build."
	detail := "Enable research.provider=tavily to register web_search."
	if active {
		summary = "web_search is registered."
		detail = fmt.Sprintf("%d results · %ds timeout", cfg.Research.MaxResults, cfg.Research.TimeoutSec)
	}
	return extensionstatus.Surface{
		ID:                   "tavily",
		Name:                 "Tavily research",
		Kind:                 "research",
		Configured:           active,
		Active:               active,
		CredentialState:      extensionCredentialState(ready, active),
		CredentialStateLabel: extensionCredentialStateLabel(extensionCredentialState(ready, active)),
		Summary:              summary,
		Detail:               detail,
	}
}

func extensionMCPSurfaces(cfg Config) []extensionstatus.Surface {
	if len(cfg.MCP.Servers) == 0 {
		return nil
	}

	surfaces := make([]extensionstatus.Surface, 0, len(cfg.MCP.Servers))
	for _, server := range cfg.MCP.Servers {
		enabled := 0
		for _, tool := range server.Tools {
			if tool.Enabled {
				enabled++
			}
		}
		command := strings.Join(server.Command, " ")
		if command == "" {
			command = "command not configured"
		}
		surfaces = append(surfaces, extensionstatus.Surface{
			ID:                   server.ID,
			Name:                 server.ID,
			Kind:                 "mcp",
			Configured:           len(server.Command) > 0,
			Active:               enabled > 0,
			CredentialState:      "operator_managed",
			CredentialStateLabel: extensionCredentialStateLabel("operator_managed"),
			Summary:              fmt.Sprintf("%d MCP tools enabled.", enabled),
			Detail:               strings.TrimSpace(strings.Join([]string{server.Transport, command}, " · ")),
		})
	}
	sort.Slice(surfaces, func(i, j int) bool { return surfaces[i].Name < surfaces[j].Name })
	return surfaces
}

func extensionConnectorSurface(
	id string,
	name string,
	configured bool,
	ready bool,
	agentID string,
	health model.ConnectorHealthSnapshot,
	meta model.ConnectorMetadata,
) extensionstatus.Surface {
	active := meta.ID != ""
	summary := connectorSurfaceSummary(configured, health)
	detail := connectorSurfaceDetail(agentID, meta)
	return extensionstatus.Surface{
		ID:                   id,
		Name:                 name,
		Kind:                 "connector",
		Configured:           configured,
		Active:               active,
		CredentialState:      extensionCredentialState(ready, configured),
		CredentialStateLabel: extensionCredentialStateLabel(extensionCredentialState(ready, configured)),
		Summary:              summary,
		Detail:               detail,
	}
}

func extensionZaloSurface(cfg Config, ready bool, health model.ConnectorHealthSnapshot, meta model.ConnectorMetadata) extensionstatus.Surface {
	configured := cfg.ZaloPersonal.Enabled
	active := meta.ID != ""
	summary := connectorSurfaceSummary(configured, health)
	if configured && !ready {
		summary = "Runtime is enabled, but browser auth still depends on stored credentials."
	}
	detail := strings.TrimSpace(strings.Join([]string{
		"reply mode " + cfg.ZaloPersonal.Groups.ReplyMode,
		connectorSurfaceDetail(cfg.ZaloPersonal.AgentID, meta),
	}, " · "))
	return extensionstatus.Surface{
		ID:                   "zalo_personal",
		Name:                 "Zalo Personal",
		Kind:                 "connector",
		Configured:           configured,
		Active:               active,
		CredentialState:      extensionCredentialState(ready, configured),
		CredentialStateLabel: extensionCredentialStateLabel(extensionCredentialState(ready, configured)),
		Summary:              summary,
		Detail:               detail,
	}
}

func connectorSurfaceSummary(configured bool, health model.ConnectorHealthSnapshot) string {
	if strings.TrimSpace(health.Summary) != "" {
		return health.Summary
	}
	if configured {
		return "Connector is configured."
	}
	return "Connector is not configured."
}

func connectorSurfaceDetail(agentID string, meta model.ConnectorMetadata) string {
	parts := make([]string, 0, 3)
	if trimmed := strings.TrimSpace(agentID); trimmed != "" {
		parts = append(parts, "Front agent "+trimmed)
	}
	if meta.ID != "" && meta.Exposure != "" {
		parts = append(parts, string(meta.Exposure))
	}
	if len(meta.Aliases) > 0 {
		parts = append(parts, "aliases "+strings.Join(meta.Aliases, ", "))
	}
	if len(parts) == 0 {
		return "Operator-managed through runtime config."
	}
	return strings.Join(parts, " · ")
}

func extensionCredentialState(ready bool, configured bool) string {
	if ready {
		return "ready"
	}
	if configured {
		return "missing"
	}
	return "unused"
}

func extensionCredentialStateLabel(state string) string {
	return strings.ReplaceAll(state, "_", " ")
}

func (a *App) loadExtensionTools() []extensionstatus.Tool {
	if a.toolRegistry == nil {
		return nil
	}

	mcpTools := make(map[string]bool)
	for _, server := range a.cfg.MCP.Servers {
		for _, tool := range server.Tools {
			if tool.Enabled {
				mcpTools[tool.Alias] = true
			}
		}
	}

	specs := a.toolRegistry.List()
	items := make([]extensionstatus.Tool, 0, len(specs))
	for _, spec := range specs {
		items = append(items, extensionstatus.Tool{
			Name:        spec.Name,
			Family:      extensionToolFamily(spec, mcpTools),
			Risk:        string(spec.Risk),
			Approval:    spec.Approval,
			SideEffect:  spec.SideEffect,
			Description: spec.Description,
		})
	}
	return items
}

func extensionToolFamily(spec model.ToolSpec, mcpTools map[string]bool) string {
	if mcpTools[spec.Name] {
		return "mcp"
	}
	if spec.Name == "web_search" {
		return "research"
	}
	if spec.Name == "web_fetch" {
		return "web"
	}

	switch spec.Family {
	case model.ToolFamilyRepoRead, model.ToolFamilyRepoWrite, model.ToolFamilyDiffReview, model.ToolFamilyVerification:
		return "repo"
	case model.ToolFamilyConnectorCapability:
		return "connector"
	case model.ToolFamilyRuntimeCapability:
		return "app"
	case model.ToolFamilyDelegate:
		return "collaboration"
	case model.ToolFamilyWebRead:
		return "web"
	default:
		return strings.ReplaceAll(string(spec.Family), "_", " ")
	}
}

func summarizeExtensionStatus(status extensionstatus.Status) extensionstatus.Summary {
	summary := extensionstatus.Summary{
		ShippedSurfaces: len(status.Surfaces),
		InstalledTools:  len(status.Tools),
	}
	for _, surface := range status.Surfaces {
		if surface.Configured {
			summary.ConfiguredSurfaces++
		}
		if surface.CredentialState == "ready" {
			summary.ReadyCredentials++
		}
		if surface.CredentialState == "missing" {
			summary.MissingCredentials++
		}
	}
	return summary
}
