package app

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/canhta/gistclaw/internal/connectors/zalopersonal"
)

type stubZaloPersonalQRRunner struct {
	creds   zalopersonal.StoredCredentials
	qrPNG   []byte
	err     error
	calls   int
	lastCtx context.Context
}

func (s *stubZaloPersonalQRRunner) LoginQR(ctx context.Context, qrCallback func([]byte)) (zalopersonal.StoredCredentials, error) {
	s.calls++
	s.lastCtx = ctx
	if len(s.qrPNG) > 0 && qrCallback != nil {
		qrCallback(s.qrPNG)
	}
	if s.err != nil {
		return zalopersonal.StoredCredentials{}, s.err
	}
	return s.creds, nil
}

type stubZaloPersonalFriendsReader struct {
	friends   []ZaloPersonalFriend
	err       error
	calls     int
	lastCtx   context.Context
	lastCreds zalopersonal.StoredCredentials
}

func (s *stubZaloPersonalFriendsReader) ListFriends(ctx context.Context, creds zalopersonal.StoredCredentials) ([]ZaloPersonalFriend, error) {
	s.calls++
	s.lastCtx = ctx
	s.lastCreds = creds
	if s.err != nil {
		return nil, s.err
	}
	return s.friends, nil
}

func TestApp_ZaloPersonalAuth(t *testing.T) {
	ctx := context.Background()

	t.Run("login saves returned credentials and forwards qr callback", func(t *testing.T) {
		app := setupCommandApp(t)
		runner := &stubZaloPersonalQRRunner{
			creds: zalopersonal.StoredCredentials{
				AccountID:   "123456789",
				DisplayName: "Canh",
				IMEI:        "imei-123",
				Cookie:      "cookie=value",
				UserAgent:   "Mozilla/5.0",
				Language:    "vi",
			},
			qrPNG: []byte("png-bytes"),
		}

		var qr bytes.Buffer
		got, err := app.LoginZaloPersonalQR(ctx, runner, func(png []byte) {
			_, _ = qr.Write(png)
		})
		if err != nil {
			t.Fatalf("LoginZaloPersonalQR: %v", err)
		}
		if runner.calls != 1 {
			t.Fatalf("expected runner to be called once, got %d", runner.calls)
		}
		if got != runner.creds {
			t.Fatalf("expected creds %+v, got %+v", runner.creds, got)
		}
		if !bytes.Equal(qr.Bytes(), runner.qrPNG) {
			t.Fatalf("expected qr callback bytes %q, got %q", runner.qrPNG, qr.Bytes())
		}

		stored, ok, err := app.ZaloPersonalStoredCredentials(ctx)
		if err != nil {
			t.Fatalf("ZaloPersonalStoredCredentials: %v", err)
		}
		if !ok {
			t.Fatal("expected stored credentials after login")
		}
		if stored != runner.creds {
			t.Fatalf("expected stored creds %+v, got %+v", runner.creds, stored)
		}
	})

	t.Run("login runner error leaves storage empty", func(t *testing.T) {
		app := setupCommandApp(t)
		runner := &stubZaloPersonalQRRunner{err: fmt.Errorf("boom")}

		if _, err := app.LoginZaloPersonalQR(ctx, runner, nil); err == nil {
			t.Fatal("expected LoginZaloPersonalQR to fail")
		}

		_, ok, err := app.ZaloPersonalStoredCredentials(ctx)
		if err != nil {
			t.Fatalf("ZaloPersonalStoredCredentials: %v", err)
		}
		if ok {
			t.Fatal("expected no stored credentials after failed login")
		}
	})

	t.Run("clear removes stored credentials", func(t *testing.T) {
		app := setupCommandApp(t)
		if err := zalopersonal.SaveStoredCredentials(ctx, app.db, zalopersonal.StoredCredentials{
			AccountID: "123456789",
			IMEI:      "imei-123",
			Cookie:    "cookie=value",
			UserAgent: "Mozilla/5.0",
		}); err != nil {
			t.Fatalf("SaveStoredCredentials: %v", err)
		}

		if err := app.ClearZaloPersonalCredentials(ctx); err != nil {
			t.Fatalf("ClearZaloPersonalCredentials: %v", err)
		}

		_, ok, err := app.ZaloPersonalStoredCredentials(ctx)
		if err != nil {
			t.Fatalf("ZaloPersonalStoredCredentials: %v", err)
		}
		if ok {
			t.Fatal("expected no stored credentials after clear")
		}
	})

	t.Run("list friends uses stored credentials", func(t *testing.T) {
		app := setupCommandApp(t)
		creds := zalopersonal.StoredCredentials{
			AccountID:   "123456789",
			DisplayName: "Canh",
			IMEI:        "imei-123",
			Cookie:      "cookie=value",
			UserAgent:   "Mozilla/5.0",
			Language:    "vi",
		}
		if err := zalopersonal.SaveStoredCredentials(ctx, app.db, creds); err != nil {
			t.Fatalf("SaveStoredCredentials: %v", err)
		}
		reader := &stubZaloPersonalFriendsReader{
			friends: []ZaloPersonalFriend{
				{UserID: "user-1", DisplayName: "An"},
				{UserID: "user-2", DisplayName: "Bao"},
			},
		}

		got, err := app.ListZaloPersonalFriends(ctx, reader)
		if err != nil {
			t.Fatalf("ListZaloPersonalFriends: %v", err)
		}
		if reader.calls != 1 {
			t.Fatalf("expected reader to be called once, got %d", reader.calls)
		}
		if reader.lastCreds != creds {
			t.Fatalf("expected creds %+v, got %+v", creds, reader.lastCreds)
		}
		if len(got) != 2 || got[0].UserID != "user-1" || got[1].UserID != "user-2" {
			t.Fatalf("unexpected friends: %+v", got)
		}
	})

	t.Run("list friends without stored credentials fails", func(t *testing.T) {
		app := setupCommandApp(t)
		reader := &stubZaloPersonalFriendsReader{}

		if _, err := app.ListZaloPersonalFriends(ctx, reader); err == nil {
			t.Fatal("expected ListZaloPersonalFriends to fail without stored credentials")
		}
		if reader.calls != 0 {
			t.Fatalf("expected reader to stay unused, got %d calls", reader.calls)
		}
	})
}
