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

const authZaloPersonalUsage = "Usage: gistclaw auth zalo-personal <login|logout|contacts|groups|send-text|send-image|send-file>"

var (
	newZaloPersonalQRRunner                  = func() app.ZaloPersonalQRLoginRunner { return zaloPersonalProtocolQRRunner{} }
	newZaloPersonalFriendsReader             = func() app.ZaloPersonalFriendsReader { return zaloPersonalProtocolFriendsReader{} }
	newZaloPersonalGroupsReader              = func() app.ZaloPersonalGroupsReader { return zaloPersonalProtocolGroupsReader{} }
	zaloPersonalProtocolLoginQR              = protocol.LoginQR
	zaloPersonalProtocolLoginWithCredentials = protocol.LoginWithCredentials
	zaloPersonalProtocolFetchFriends         = protocol.FetchFriends
	zaloPersonalProtocolFetchGroups          = protocol.FetchGroups
	zaloPersonalProtocolSendMessage          = protocol.SendMessage
	zaloPersonalProtocolUploadImage          = protocol.UploadImage
	zaloPersonalProtocolSendImage            = protocol.SendImage
	zaloPersonalProtocolUploadFile           = func(ctx context.Context, sess *protocol.Session, threadID string, threadType protocol.ThreadType, filePath string) (*protocol.FileUploadResult, error) {
		listener, err := protocol.NewListener(sess)
		if err != nil {
			return nil, err
		}
		if err := listener.Start(ctx); err != nil {
			return nil, err
		}
		defer listener.Stop()
		return protocol.UploadFile(ctx, sess, listener, threadID, threadType, filePath)
	}
	zaloPersonalProtocolSendFile = protocol.SendFile
	zaloPersonalLoginTimeout     = 2 * time.Minute
	zaloPersonalContactsTimeout  = 30 * time.Second
)

type zaloPersonalProtocolQRRunner struct{}
type zaloPersonalProtocolFriendsReader struct{}
type zaloPersonalProtocolGroupsReader struct{}

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

	friends, err := zaloPersonalProtocolFetchFriends(ctx, sess)
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

func (zaloPersonalProtocolGroupsReader) ListGroups(ctx context.Context, creds zalopersonal.StoredCredentials) ([]app.ZaloPersonalGroup, error) {
	sess, err := zaloPersonalProtocolLoginWithCredentials(ctx, protocol.Credentials{
		IMEI:      creds.IMEI,
		Cookie:    creds.Cookie,
		UserAgent: creds.UserAgent,
		Language:  zaloPersonalLanguagePtr(creds.Language),
	})
	if err != nil {
		return nil, err
	}

	groups, err := zaloPersonalProtocolFetchGroups(ctx, sess)
	if err != nil {
		return nil, err
	}

	results := make([]app.ZaloPersonalGroup, 0, len(groups))
	for _, group := range groups {
		results = append(results, app.ZaloPersonalGroup{
			GroupID:     strings.TrimSpace(group.GroupID),
			Name:        strings.TrimSpace(group.Name),
			Avatar:      strings.TrimSpace(group.Avatar),
			TotalMember: group.TotalMember,
		})
	}
	return results, nil
}

func runAuthZaloPersonal(opts globalOptions, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, authZaloPersonalUsage)
		return 1
	}

	switch args[0] {
	case "login":
		if len(args) != 1 {
			fmt.Fprintln(stderr, authZaloPersonalUsage)
			return 1
		}
		return runAuthZaloPersonalLogin(opts, stdout, stderr)
	case "logout":
		if len(args) != 1 {
			fmt.Fprintln(stderr, authZaloPersonalUsage)
			return 1
		}
		return runAuthZaloPersonalLogout(opts, stdout, stderr)
	case "contacts":
		if len(args) != 1 {
			fmt.Fprintln(stderr, authZaloPersonalUsage)
			return 1
		}
		return runAuthZaloPersonalContacts(opts, stdout, stderr)
	case "groups":
		if len(args) != 1 {
			fmt.Fprintln(stderr, authZaloPersonalUsage)
			return 1
		}
		return runAuthZaloPersonalGroups(opts, stdout, stderr)
	case "send-text":
		return runAuthZaloPersonalSendText(opts, args[1:], stdout, stderr)
	case "send-image":
		return runAuthZaloPersonalSendImage(opts, args[1:], stdout, stderr)
	case "send-file":
		return runAuthZaloPersonalSendFile(opts, args[1:], stdout, stderr)
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

func runAuthZaloPersonalGroups(opts globalOptions, stdout, stderr io.Writer) int {
	application, err := loadApp(opts)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
		return 1
	}
	defer func() { _ = application.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), zaloPersonalContactsTimeout)
	defer cancel()

	groups, err := application.ListZaloPersonalGroups(ctx, newZaloPersonalGroupsReader())
	if err != nil {
		fmt.Fprintf(stderr, "auth zalo-personal groups failed: %v\n", err)
		return 1
	}

	sort.Slice(groups, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(groups[i].Name))
		right := strings.ToLower(strings.TrimSpace(groups[j].Name))
		if left == right {
			return groups[i].GroupID < groups[j].GroupID
		}
		return left < right
	})

	fmt.Fprintln(stdout, "group_id\tname\ttotal_member")
	for _, group := range groups {
		fmt.Fprintf(
			stdout,
			"%s\t%s\t%d\n",
			sanitizeZaloTabField(group.GroupID),
			sanitizeZaloTabField(group.Name),
			group.TotalMember,
		)
	}
	return 0
}

func runAuthZaloPersonalSendText(opts globalOptions, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "Usage: gistclaw auth zalo-personal send-text <chat-id> <text>")
		return 1
	}
	cfg, creds, sess, ok := loadZaloPersonalSessionForSend(opts, stderr)
	if !ok {
		return 1
	}
	threadType := zaloPersonalThreadType(cfg, args[0])
	if _, err := zaloPersonalProtocolSendMessage(context.Background(), sess, args[0], threadType, args[1]); err != nil {
		fmt.Fprintf(stderr, "auth zalo-personal send-text failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "sent text to %s (%s)\n", args[0], creds.AccountID)
	return 0
}

func runAuthZaloPersonalSendImage(opts globalOptions, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 && len(args) != 3 {
		fmt.Fprintln(stderr, "Usage: gistclaw auth zalo-personal send-image <chat-id> <file-path> [caption]")
		return 1
	}
	cfg, creds, sess, ok := loadZaloPersonalSessionForSend(opts, stderr)
	if !ok {
		return 1
	}
	threadType := zaloPersonalThreadType(cfg, args[0])
	upload, err := zaloPersonalProtocolUploadImage(context.Background(), sess, args[0], threadType, args[1])
	if err != nil {
		fmt.Fprintf(stderr, "auth zalo-personal send-image failed: %v\n", err)
		return 1
	}
	caption := ""
	if len(args) == 3 {
		caption = args[2]
	}
	if _, err := zaloPersonalProtocolSendImage(context.Background(), sess, args[0], threadType, upload, caption); err != nil {
		fmt.Fprintf(stderr, "auth zalo-personal send-image failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "sent image to %s (%s)\n", args[0], creds.AccountID)
	return 0
}

func runAuthZaloPersonalSendFile(opts globalOptions, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "Usage: gistclaw auth zalo-personal send-file <chat-id> <file-path>")
		return 1
	}
	cfg, creds, sess, ok := loadZaloPersonalSessionForSend(opts, stderr)
	if !ok {
		return 1
	}
	threadType := zaloPersonalThreadType(cfg, args[0])
	upload, err := zaloPersonalProtocolUploadFile(context.Background(), sess, args[0], threadType, args[1])
	if err != nil {
		fmt.Fprintf(stderr, "auth zalo-personal send-file failed: %v\n", err)
		return 1
	}
	if _, err := zaloPersonalProtocolSendFile(context.Background(), sess, args[0], threadType, upload); err != nil {
		fmt.Fprintf(stderr, "auth zalo-personal send-file failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "sent file to %s (%s)\n", args[0], creds.AccountID)
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

func loadZaloPersonalSessionForSend(opts globalOptions, stderr io.Writer) (app.Config, zalopersonal.StoredCredentials, *protocol.Session, bool) {
	cfg, err := loadConfigRawWithOverrides(opts)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return app.Config{}, zalopersonal.StoredCredentials{}, nil, false
	}

	application, err := loadApp(opts)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap app: %v\n", err)
		return app.Config{}, zalopersonal.StoredCredentials{}, nil, false
	}
	defer func() { _ = application.Stop() }()

	creds, ok, err := application.ZaloPersonalStoredCredentials(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "load stored credentials: %v\n", err)
		return app.Config{}, zalopersonal.StoredCredentials{}, nil, false
	}
	if !ok {
		fmt.Fprintln(stderr, "load stored credentials: not authenticated")
		return app.Config{}, zalopersonal.StoredCredentials{}, nil, false
	}

	sess, err := zaloPersonalProtocolLoginWithCredentials(context.Background(), protocol.Credentials{
		IMEI:      creds.IMEI,
		Cookie:    creds.Cookie,
		UserAgent: creds.UserAgent,
		Language:  zaloPersonalLanguagePtr(creds.Language),
	})
	if err != nil {
		fmt.Fprintf(stderr, "open zalo session: %v\n", err)
		return app.Config{}, zalopersonal.StoredCredentials{}, nil, false
	}
	return cfg, creds, sess, true
}

func zaloPersonalThreadType(cfg app.Config, chatID string) protocol.ThreadType {
	for _, groupID := range cfg.ZaloPersonal.Groups.Allowlist {
		if strings.TrimSpace(groupID) == strings.TrimSpace(chatID) {
			return protocol.ThreadTypeGroup
		}
	}
	return protocol.ThreadTypeUser
}
