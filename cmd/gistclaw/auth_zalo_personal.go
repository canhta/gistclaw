package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/canhta/gistclaw/internal/app"
	"github.com/canhta/gistclaw/internal/connectors/zalopersonal"
	"github.com/canhta/gistclaw/internal/connectors/zalopersonal/protocol"
)

const authZaloPersonalUsage = "Usage: gistclaw auth zalo-personal <login|logout|contacts>"

var (
	newZaloPersonalQRRunner                  = func() app.ZaloPersonalQRLoginRunner { return zaloPersonalProtocolQRRunner{} }
	newZaloPersonalFriendsReader             = func() app.ZaloPersonalFriendsReader { return zaloPersonalProtocolFriendsReader{} }
	zaloPersonalProtocolLoginQR              = protocol.LoginQR
	zaloPersonalProtocolLoginWithCredentials = protocol.LoginWithCredentials
	zaloPersonalLoginTimeout                 = 2 * time.Minute
	zaloPersonalContactsTimeout              = 30 * time.Second
)

type zaloPersonalProtocolQRRunner struct{}
type zaloPersonalProtocolFriendsReader struct{}

func (zaloPersonalProtocolQRRunner) LoginQR(ctx context.Context, qrCallback func([]byte)) (zalopersonal.StoredCredentials, error) {
	creds, err := zaloPersonalProtocolLoginQR(ctx, qrCallback)
	if err != nil {
		return zalopersonal.StoredCredentials{}, err
	}
	session, err := zaloPersonalProtocolLoginWithCredentials(ctx, *creds)
	if err != nil {
		return zalopersonal.StoredCredentials{}, err
	}
	if strings.TrimSpace(session.UID) == "" {
		return zalopersonal.StoredCredentials{}, fmt.Errorf("zalo personal auth: missing account id after login")
	}

	stored := zalopersonal.StoredCredentials{
		AccountID:   session.UID,
		DisplayName: strings.TrimSpace(creds.DisplayName),
		IMEI:        creds.IMEI,
		Cookie:      creds.Cookie,
		UserAgent:   creds.UserAgent,
	}
	if creds.Language != nil {
		stored.Language = strings.TrimSpace(*creds.Language)
	}
	return stored, nil
}

func (zaloPersonalProtocolFriendsReader) ListFriends(ctx context.Context, creds zalopersonal.StoredCredentials) ([]app.ZaloPersonalFriend, error) {
	sess, err := zaloPersonalProtocolLoginWithCredentials(ctx, protocol.Credentials{
		IMEI:      creds.IMEI,
		Cookie:    creds.Cookie,
		UserAgent: creds.UserAgent,
		Language:  zaloPersonalLanguagePtr(creds.Language),
	})
	if err != nil {
		return nil, err
	}

	friends, err := protocol.FetchFriends(ctx, sess)
	if err != nil {
		return nil, err
	}

	results := make([]app.ZaloPersonalFriend, 0, len(friends))
	for _, friend := range friends {
		results = append(results, app.ZaloPersonalFriend{
			UserID:      strings.TrimSpace(friend.UserID),
			DisplayName: strings.TrimSpace(friend.DisplayName),
			ZaloName:    strings.TrimSpace(friend.ZaloName),
			Avatar:      strings.TrimSpace(friend.Avatar),
		})
	}
	return results, nil
}

func runAuthZaloPersonal(opts globalOptions, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, authZaloPersonalUsage)
		return 1
	}

	switch args[0] {
	case "login":
		return runAuthZaloPersonalLogin(opts, stdout, stderr)
	case "logout":
		return runAuthZaloPersonalLogout(opts, stdout, stderr)
	case "contacts":
		return runAuthZaloPersonalContacts(opts, stdout, stderr)
	default:
		fmt.Fprintln(stderr, authZaloPersonalUsage)
		return 1
	}
}

func runAuthZaloPersonalLogin(opts globalOptions, stdout, stderr io.Writer) int {
	cfg, err := loadConfigRawWithOverrides(opts)
	if err != nil {
		fmt.Fprintf(stderr, "auth zalo-personal login failed: %v\n", err)
		return 1
	}

	application, err := loadApp(opts)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
		return 1
	}
	defer func() { _ = application.Stop() }()

	qrPath := filepath.Join(cfg.StateDir, "auth", "zalo-personal-qr.png")
	var qrWriteErr error
	var qrWritten bool
	ctx, cancel := context.WithTimeout(context.Background(), zaloPersonalLoginTimeout)
	defer cancel()

	_, err = application.LoginZaloPersonalQR(ctx, newZaloPersonalQRRunner(), func(png []byte) {
		if qrWriteErr != nil {
			return
		}
		if err := os.MkdirAll(filepath.Dir(qrPath), 0o700); err != nil {
			qrWriteErr = err
			return
		}
		if err := os.WriteFile(qrPath, png, 0o600); err != nil {
			qrWriteErr = err
			return
		}
		if !qrWritten {
			fmt.Fprintf(stdout, "Scan QR image: %s\n", qrPath)
		}
		qrWritten = true
	})
	if qrWriteErr != nil {
		fmt.Fprintf(stderr, "auth zalo-personal login failed: write qr file: %v\n", qrWriteErr)
		return 1
	}
	if err != nil {
		fmt.Fprintf(stderr, "auth zalo-personal login failed: %v\n", err)
		return 1
	}
	if !qrWritten {
		fmt.Fprintln(stderr, "auth zalo-personal login failed: qr code not emitted")
		return 1
	}
	return 0
}

func runAuthZaloPersonalLogout(opts globalOptions, stdout, stderr io.Writer) int {
	application, err := loadApp(opts)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
		return 1
	}
	defer func() { _ = application.Stop() }()

	if err := application.ClearZaloPersonalCredentials(context.Background()); err != nil {
		fmt.Fprintf(stderr, "auth zalo-personal logout failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "zalo personal credentials cleared")
	return 0
}

func runAuthZaloPersonalContacts(opts globalOptions, stdout, stderr io.Writer) int {
	application, err := loadApp(opts)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
		return 1
	}
	defer func() { _ = application.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), zaloPersonalContactsTimeout)
	defer cancel()

	friends, err := application.ListZaloPersonalFriends(ctx, newZaloPersonalFriendsReader())
	if err != nil {
		fmt.Fprintf(stderr, "auth zalo-personal contacts failed: %v\n", err)
		return 1
	}

	sort.Slice(friends, func(i, j int) bool {
		left := strings.ToLower(zaloPersonalFriendLabel(friends[i]))
		right := strings.ToLower(zaloPersonalFriendLabel(friends[j]))
		if left == right {
			return friends[i].UserID < friends[j].UserID
		}
		return left < right
	})

	fmt.Fprintln(stdout, "user_id\tdisplay_name")
	for _, friend := range friends {
		fmt.Fprintf(stdout, "%s\t%s\n", sanitizeZaloTabField(friend.UserID), sanitizeZaloTabField(zaloPersonalFriendLabel(friend)))
	}
	return 0
}

func zaloPersonalLanguagePtr(language string) *string {
	language = strings.TrimSpace(language)
	if language == "" {
		return nil
	}
	return &language
}

func zaloPersonalFriendLabel(friend app.ZaloPersonalFriend) string {
	if name := strings.TrimSpace(friend.DisplayName); name != "" {
		return name
	}
	if name := strings.TrimSpace(friend.ZaloName); name != "" {
		return name
	}
	return friend.UserID
}

func sanitizeZaloTabField(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.TrimSpace(value)
}
