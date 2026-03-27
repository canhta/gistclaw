package app

import (
	"context"
	"fmt"

	"github.com/canhta/gistclaw/internal/connectors/zalopersonal"
)

type ZaloPersonalQRLoginRunner interface {
	LoginQR(ctx context.Context, qrCallback func([]byte)) (zalopersonal.StoredCredentials, error)
}

type ZaloPersonalFriend struct {
	UserID      string
	DisplayName string
	ZaloName    string
	Avatar      string
}

type ZaloPersonalGroup struct {
	GroupID     string
	Name        string
	Avatar      string
	TotalMember int
}

type ZaloPersonalFriendsReader interface {
	ListFriends(ctx context.Context, creds zalopersonal.StoredCredentials) ([]ZaloPersonalFriend, error)
}

type ZaloPersonalGroupsReader interface {
	ListGroups(ctx context.Context, creds zalopersonal.StoredCredentials) ([]ZaloPersonalGroup, error)
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

func (a *App) ListZaloPersonalFriends(ctx context.Context, reader ZaloPersonalFriendsReader) ([]ZaloPersonalFriend, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("zalo personal contacts: db is required")
	}
	if reader == nil {
		return nil, fmt.Errorf("zalo personal contacts: reader is required")
	}

	creds, ok, err := zalopersonal.LoadStoredCredentials(ctx, a.db)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("zalo personal contacts: not authenticated")
	}

	friends, err := reader.ListFriends(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("zalo personal contacts: %w", err)
	}
	return friends, nil
}

func (a *App) ListZaloPersonalGroups(ctx context.Context, reader ZaloPersonalGroupsReader) ([]ZaloPersonalGroup, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("zalo personal groups: db is required")
	}
	if reader == nil {
		return nil, fmt.Errorf("zalo personal groups: reader is required")
	}

	creds, ok, err := zalopersonal.LoadStoredCredentials(ctx, a.db)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("zalo personal groups: not authenticated")
	}

	groups, err := reader.ListGroups(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("zalo personal groups: %w", err)
	}
	return groups, nil
}
