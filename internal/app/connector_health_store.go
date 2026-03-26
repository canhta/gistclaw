package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/store"
)

const (
	connectorHealthSettingPrefix   = "connector_health."
	connectorHealthSnapshotMaxAge  = 30 * time.Second
	connectorHealthPersistInterval = 5 * time.Second
)

func connectorHealthSettingKey(connectorID string) string {
	return connectorHealthSettingPrefix + connectorID
}

func storeConnectorHealthSnapshots(ctx context.Context, db *store.DB, snapshots []model.ConnectorHealthSnapshot) error {
	if db == nil || len(snapshots) == 0 {
		return nil
	}

	return db.Tx(ctx, func(tx *sql.Tx) error {
		for _, snapshot := range snapshots {
			connectorID := strings.TrimSpace(snapshot.ConnectorID)
			if connectorID == "" {
				continue
			}
			raw, err := json.Marshal(snapshot)
			if err != nil {
				return fmt.Errorf("marshal %s snapshot: %w", connectorID, err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
				 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
				connectorHealthSettingKey(connectorID),
				string(raw),
			); err != nil {
				return fmt.Errorf("store %s snapshot: %w", connectorID, err)
			}
		}
		return nil
	})
}

func loadRecentConnectorHealthSnapshots(ctx context.Context, db *store.DB, now time.Time, maxAge time.Duration) (map[string]model.ConnectorHealthSnapshot, error) {
	if db == nil {
		return nil, fmt.Errorf("connector health: db is required")
	}
	if maxAge <= 0 {
		maxAge = connectorHealthSnapshotMaxAge
	}

	rows, err := db.RawDB().QueryContext(ctx,
		`SELECT key, value
		 FROM settings
		 WHERE key LIKE ?`,
		connectorHealthSettingPrefix+"%",
	)
	if err != nil {
		if strings.Contains(err.Error(), "no such table: settings") {
			return map[string]model.ConnectorHealthSnapshot{}, nil
		}
		return nil, fmt.Errorf("query persisted connector health: %w", err)
	}
	defer rows.Close()

	snapshots := make(map[string]model.ConnectorHealthSnapshot)
	for rows.Next() {
		var (
			key string
			raw string
		)
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, fmt.Errorf("scan persisted connector health: %w", err)
		}

		connectorID := strings.TrimPrefix(key, connectorHealthSettingPrefix)
		if connectorID == "" {
			continue
		}

		var snapshot model.ConnectorHealthSnapshot
		if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
			return nil, fmt.Errorf("decode persisted connector health for %s: %w", connectorID, err)
		}
		snapshot.ConnectorID = connectorID
		if snapshot.CheckedAt.IsZero() {
			continue
		}
		snapshot.CheckedAt = snapshot.CheckedAt.UTC()
		if now.Sub(snapshot.CheckedAt) > maxAge {
			continue
		}
		snapshots[connectorID] = snapshot
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate persisted connector health: %w", err)
	}

	return snapshots, nil
}
