package zalopersonal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/canhta/gistclaw/internal/store"
)

const (
	settingCredentials = "zalo_personal_credentials"
	settingAccountID   = "zalo_personal_account_id"
	settingDisplayName = "zalo_personal_display_name"
)

func LoadStoredCredentials(ctx context.Context, db *store.DB) (StoredCredentials, bool, error) {
	if db == nil {
		return StoredCredentials{}, false, fmt.Errorf("zalo personal: db is required")
	}

	raw, err := lookupSetting(ctx, db, settingCredentials)
	if err != nil {
		return StoredCredentials{}, false, err
	}
	if raw == "" {
		return StoredCredentials{}, false, nil
	}

	var creds StoredCredentials
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return StoredCredentials{}, false, fmt.Errorf("zalo personal: decode stored credentials: %w", err)
	}
	creds = creds.normalized()

	if creds.AccountID == "" {
		accountID, err := lookupSetting(ctx, db, settingAccountID)
		if err != nil {
			return StoredCredentials{}, false, err
		}
		creds.AccountID = accountID
	}
	if creds.DisplayName == "" {
		displayName, err := lookupSetting(ctx, db, settingDisplayName)
		if err != nil {
			return StoredCredentials{}, false, err
		}
		creds.DisplayName = displayName
	}

	return creds.normalized(), true, nil
}

func SaveStoredCredentials(ctx context.Context, db *store.DB, creds StoredCredentials) error {
	if db == nil {
		return fmt.Errorf("zalo personal: db is required")
	}
	creds = creds.normalized()
	if err := creds.Validate(); err != nil {
		return err
	}

	payload, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("zalo personal: marshal stored credentials: %w", err)
	}

	return db.Tx(ctx, func(tx *sql.Tx) error {
		if err := upsertSetting(ctx, tx, settingCredentials, string(payload)); err != nil {
			return err
		}
		if err := upsertSetting(ctx, tx, settingAccountID, creds.AccountID); err != nil {
			return err
		}
		if err := upsertSetting(ctx, tx, settingDisplayName, creds.DisplayName); err != nil {
			return err
		}
		return nil
	})
}

func ClearStoredCredentials(ctx context.Context, db *store.DB) error {
	if db == nil {
		return fmt.Errorf("zalo personal: db is required")
	}
	return db.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM settings WHERE key IN (?, ?, ?)`,
			settingCredentials,
			settingAccountID,
			settingDisplayName,
		); err != nil {
			return fmt.Errorf("zalo personal: clear stored credentials: %w", err)
		}
		return nil
	})
}

func lookupSetting(ctx context.Context, db *store.DB, key string) (string, error) {
	var value string
	err := db.RawDB().QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("zalo personal: load setting %q: %w", key, err)
	}
	return value, nil
}

func upsertSetting(ctx context.Context, tx *sql.Tx, key, value string) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key,
		value,
	); err != nil {
		return fmt.Errorf("zalo personal: upsert setting %q: %w", key, err)
	}
	return nil
}
