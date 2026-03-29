package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/nodeinventory"
	"github.com/canhta/gistclaw/internal/runtime"
)

func (a *App) NodeInventoryStatus(ctx context.Context) (nodeinventory.Status, error) {
	connectors, err := a.loadNodeConnectors(ctx)
	if err != nil {
		return nodeinventory.Status{}, err
	}
	runs, err := a.loadNodeRuns(ctx, 12)
	if err != nil {
		return nodeinventory.Status{}, err
	}
	capabilities := a.loadDirectCapabilities()

	summary := nodeinventory.Summary{
		Connectors:   len(connectors),
		RunNodes:     len(runs),
		Capabilities: len(capabilities),
	}
	for _, connector := range connectors {
		if connector.State == string(model.ConnectorHealthHealthy) {
			summary.HealthyConnectors++
		}
	}
	for _, run := range runs {
		if run.Status == "needs_approval" {
			summary.ApprovalNodes++
		}
	}

	return nodeinventory.Status{
		Summary:      summary,
		Connectors:   connectors,
		Runs:         runs,
		Capabilities: capabilities,
	}, nil
}

func (a *App) loadNodeConnectors(ctx context.Context) ([]nodeinventory.ConnectorStatus, error) {
	health, err := a.ConnectorHealth(ctx)
	if err != nil {
		return nil, fmt.Errorf("load connector health: %w", err)
	}
	healthByID := make(map[string]model.ConnectorHealthSnapshot, len(health))
	for _, snapshot := range health {
		healthByID[snapshot.ConnectorID] = snapshot
	}

	connectors := make([]nodeinventory.ConnectorStatus, 0, len(a.connectors))
	for _, connector := range a.connectors {
		meta := model.NormalizeConnectorMetadata(connector.Metadata())
		if meta.ID == "" {
			continue
		}
		snapshot := healthByID[meta.ID]
		connectors = append(connectors, nodeinventory.ConnectorStatus{
			ID:               meta.ID,
			Aliases:          append([]string(nil), meta.Aliases...),
			Exposure:         string(meta.Exposure),
			State:            string(snapshot.State),
			StateLabel:       strings.ReplaceAll(string(snapshot.State), "_", " "),
			Summary:          snapshot.Summary,
			CheckedAtLabel:   formatNodeTime(snapshot.CheckedAt),
			RestartSuggested: snapshot.RestartSuggested,
		})
	}
	sort.Slice(connectors, func(i, j int) bool { return connectors[i].ID < connectors[j].ID })
	return connectors, nil
}

func (a *App) loadNodeRuns(ctx context.Context, limit int) ([]nodeinventory.RunNode, error) {
	if limit <= 0 {
		limit = 12
	}

	project, err := runtime.ActiveProject(ctx, a.db)
	if err != nil {
		return nil, fmt.Errorf("load active project: %w", err)
	}

	rows, err := a.db.RawDB().QueryContext(
		ctx,
		`SELECT id, COALESCE(parent_run_id, ''), agent_id, status, COALESCE(objective, ''), created_at, updated_at
		   FROM runs
		  WHERE project_id = ?
		  ORDER BY updated_at DESC, id DESC
		  LIMIT ?`,
		project.ID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query node runs: %w", err)
	}
	defer rows.Close()

	runs := make([]nodeinventory.RunNode, 0, limit)
	for rows.Next() {
		var item nodeinventory.RunNode
		var status string
		var objective string
		var createdAt string
		var updatedAt string
		if err := rows.Scan(
			&item.ID,
			&item.ParentRunID,
			&item.AgentID,
			&status,
			&objective,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan node run: %w", err)
		}
		item.ShortID = shortenNodeID(item.ID)
		item.Kind = "worker"
		if item.ParentRunID == "" {
			item.Kind = "root"
		}
		item.Status = status
		item.StatusLabel = strings.ReplaceAll(status, "_", " ")
		item.ObjectivePreview = previewObjective(objective, 80)
		item.StartedAtLabel = formatNodeTimestamp(createdAt)
		item.UpdatedAtLabel = formatNodeTimestamp(updatedAt)
		runs = append(runs, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate node runs: %w", err)
	}
	return runs, nil
}

func (a *App) loadDirectCapabilities() []nodeinventory.Capability {
	if a.toolRegistry == nil {
		return nil
	}

	specs := a.toolRegistry.List()
	capabilities := make([]nodeinventory.Capability, 0, len(specs))
	for _, spec := range specs {
		if !strings.HasPrefix(spec.Name, "connector_") && spec.Name != "app_action" {
			continue
		}
		family := "connector"
		if spec.Name == "app_action" {
			family = "app"
		}
		capabilities = append(capabilities, nodeinventory.Capability{
			Name:        spec.Name,
			Family:      family,
			Description: spec.Description,
		})
	}
	return capabilities
}

func shortenNodeID(id string) string {
	trimmed := strings.TrimSpace(id)
	if len(trimmed) <= 16 {
		return trimmed
	}
	return trimmed[:8] + "…" + trimmed[len(trimmed)-4:]
}

func previewObjective(value string, limit int) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if limit <= 0 || len(trimmed) <= limit {
		return trimmed
	}
	return trimmed[:limit-1] + "…"
}

func formatNodeTimestamp(value string) string {
	parsed, err := time.Parse("2006-01-02 15:04:05", value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format("2006-01-02 15:04:05 MST")
}

func formatNodeTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02 15:04:05 MST")
}
