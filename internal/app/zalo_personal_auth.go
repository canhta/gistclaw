package app

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/connectors/zalopersonal"
)

type ZaloPersonalQRLoginRunner interface {
	LoginQR(ctx context.Context, qrCallback func([]byte)) (zalopersonal.StoredCredentials, error)
}

func (a *App) ZaloPersonalStoredCredentials(ctx context.Context) (zalopersonal.StoredCredentials, bool, error) {
	if a == nil || a.db == nil {
		return zalopersonal.StoredCredentials{}, false, fmt.Errorf("zalo personal credentials: db is required")
	}
	return zalopersonal.LoadStoredCredentials(ctx, a.db)
}

func (a *App) LoginZaloPersonalQR(ctx context.Context, runner ZaloPersonalQRLoginRunner, qrCallback func([]byte)) (zalopersonal.StoredCredentials, error) {
	if a == nil || a.db == nil {
		return zalopersonal.StoredCredentials{}, fmt.Errorf("zalo personal qr login: db is required")
	}
	if runner == nil {
		return zalopersonal.StoredCredentials{}, fmt.Errorf("zalo personal qr login: runner is required")
	}

	creds, err := runner.LoginQR(ctx, qrCallback)
	if err != nil {
		return zalopersonal.StoredCredentials{}, fmt.Errorf("zalo personal qr login: %w", err)
	}
	if err := zalopersonal.SaveStoredCredentials(ctx, a.db, creds); err != nil {
		return zalopersonal.StoredCredentials{}, err
	}
	return creds, nil
}

func (a *App) ClearZaloPersonalCredentials(ctx context.Context) error {
	if a == nil || a.db == nil {
		return fmt.Errorf("zalo personal credentials: db is required")
	}
	if err := zalopersonal.ClearStoredCredentials(ctx, a.db); err != nil {
		return err
	}
	return nil
}
