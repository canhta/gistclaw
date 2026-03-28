package zalopersonal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/canhta/gistclaw/internal/connectors/threadstate"
	"github.com/canhta/gistclaw/internal/connectors/zalopersonal/protocol"
	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime"
	"github.com/canhta/gistclaw/internal/store"
)

type ConnectorRuntime interface {
	ReceiveInboundMessage(ctx context.Context, req runtime.InboundMessageCommand) (model.Run, error)
}

type listenerSession struct {
	AccountID string
	Language  string
	Protocol  *protocol.Session
}

type SessionListener interface {
	Start(ctx context.Context) error
	Stop()
	Messages() <-chan IncomingMessage
	Errors() <-chan error
	Disconnected() <-chan error
}

type Connector struct {
	outbound               *OutboundDispatcher
	inbound                *InboundDispatcher
	health                 *HealthState
	threadState            *threadstate.Store
	login                  func(ctx context.Context, creds StoredCredentials) (*listenerSession, error)
	listFriends            func(ctx context.Context, creds StoredCredentials) ([]protocol.FriendInfo, error)
	listGroups             func(ctx context.Context, creds StoredCredentials) ([]protocol.GroupListInfo, error)
	fetchUnreadMarks       func(ctx context.Context, creds StoredCredentials) ([]protocol.UnreadMarkInfo, error)
	updateUnreadMark       func(ctx context.Context, creds StoredCredentials, threadID string, threadType protocol.ThreadType, unread bool) error
	fetchPinnedThreads     func(ctx context.Context, creds StoredCredentials) ([]protocol.PinnedConversationInfo, error)
	updatePinnedThread     func(ctx context.Context, creds StoredCredentials, threadID string, threadType protocol.ThreadType, enabled bool) error
	fetchHiddenThreads     func(ctx context.Context, creds StoredCredentials) ([]protocol.HiddenConversationInfo, error)
	updateHiddenThread     func(ctx context.Context, creds StoredCredentials, threadID string, threadType protocol.ThreadType, enabled bool) error
	fetchArchivedThreads   func(ctx context.Context, creds StoredCredentials) ([]protocol.ArchivedConversationInfo, error)
	updateArchivedThread   func(ctx context.Context, creds StoredCredentials, threadID string, threadType protocol.ThreadType, enabled bool) error
	sendText               func(ctx context.Context, creds StoredCredentials, chatID, text string) error
	newListener            func(sess *listenerSession) (SessionListener, error)
	credentialPollInterval time.Duration
	reconnectDelay         time.Duration
	duplicateSessionDelay  time.Duration
	groupPolicy            GroupPolicy
}

const maxTextChunkBytes = 2000

func NewConnector(db *store.DB, cs *conversations.ConversationStore, rt ConnectorRuntime, defaultAgentID string) *Connector {
	connector := &Connector{
		inbound:                NewInboundDispatcher(rt, defaultAgentID),
		health:                 NewHealthState(),
		threadState:            threadstate.New(db),
		credentialPollInterval: 5 * time.Second,
		reconnectDelay:         2 * time.Second,
		duplicateSessionDelay:  60 * time.Second,
	}
	connector.outbound = NewOutboundDispatcher(connector, db, cs)
	connector.login = func(ctx context.Context, creds StoredCredentials) (*listenerSession, error) {
		sess, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return nil, err
		}
		accountID := creds.AccountID
		if strings.TrimSpace(accountID) == "" {
			accountID = sess.UID
		}
		return &listenerSession{AccountID: accountID, Language: creds.Language, Protocol: sess}, nil
	}
	connector.sendText = func(ctx context.Context, creds StoredCredentials, chatID, text string) error {
		sess, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return err
		}
		_, err = protocol.SendMessage(ctx, sess, chatID, connector.threadTypeForChat(chatID), text)
		return err
	}
	connector.listFriends = func(ctx context.Context, creds StoredCredentials) ([]protocol.FriendInfo, error) {
		sess, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return nil, err
		}
		return protocol.FetchFriends(ctx, sess)
	}
	connector.listGroups = func(ctx context.Context, creds StoredCredentials) ([]protocol.GroupListInfo, error) {
		sess, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return nil, err
		}
		return protocol.FetchGroups(ctx, sess)
	}
	connector.fetchUnreadMarks = func(ctx context.Context, creds StoredCredentials) ([]protocol.UnreadMarkInfo, error) {
		sess, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return nil, err
		}
		return protocol.FetchUnreadMarks(ctx, sess)
	}
	connector.updateUnreadMark = func(ctx context.Context, creds StoredCredentials, threadID string, threadType protocol.ThreadType, unread bool) error {
		sess, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return err
		}
		return protocol.UpdateUnreadMark(ctx, sess, threadID, threadType, unread)
	}
	connector.fetchPinnedThreads = func(ctx context.Context, creds StoredCredentials) ([]protocol.PinnedConversationInfo, error) {
		sess, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return nil, err
		}
		return protocol.FetchPinnedConversations(ctx, sess)
	}
	connector.updatePinnedThread = func(ctx context.Context, creds StoredCredentials, threadID string, threadType protocol.ThreadType, enabled bool) error {
		sess, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return err
		}
		return protocol.UpdatePinnedConversation(ctx, sess, threadID, threadType, enabled)
	}
	connector.fetchHiddenThreads = func(ctx context.Context, creds StoredCredentials) ([]protocol.HiddenConversationInfo, error) {
		sess, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return nil, err
		}
		return protocol.FetchHiddenConversations(ctx, sess)
	}
	connector.updateHiddenThread = func(ctx context.Context, creds StoredCredentials, threadID string, threadType protocol.ThreadType, enabled bool) error {
		sess, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return err
		}
		return protocol.UpdateHiddenConversation(ctx, sess, threadID, threadType, enabled)
	}
	connector.fetchArchivedThreads = func(ctx context.Context, creds StoredCredentials) ([]protocol.ArchivedConversationInfo, error) {
		sess, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return nil, err
		}
		return protocol.FetchArchivedConversations(ctx, sess)
	}
	connector.updateArchivedThread = func(ctx context.Context, creds StoredCredentials, threadID string, threadType protocol.ThreadType, enabled bool) error {
		sess, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return err
		}
		return protocol.UpdateArchivedConversation(ctx, sess, threadID, threadType, enabled)
	}
	connector.newListener = func(sess *listenerSession) (SessionListener, error) {
		return newProtocolSessionListener(sess)
	}
	return connector
}

func (c *Connector) Metadata() model.ConnectorMetadata {
	return model.NormalizeConnectorMetadata(model.ConnectorMetadata{
		ID:       "zalo_personal",
		Aliases:  []string{"zalo", "zalo personal"},
		Exposure: model.ConnectorExposureRemote,
	})
}

func (c *Connector) SetGroupPolicy(policy GroupPolicy) {
	c.groupPolicy = policy
}

func (c *Connector) Start(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		creds, ok, err := LoadStoredCredentials(ctx, c.outbound.db)
		if err != nil {
			if isContextDone(err) {
				return ctx.Err()
			}
			c.health.markDisconnected("load credentials: " + err.Error())
			if err := sleepContext(ctx, c.reconnectDelay); err != nil {
				return err
			}
			continue
		}
		if !ok {
			c.health.markUnauthenticated()
			if err := sleepContext(ctx, c.credentialPollInterval); err != nil {
				return err
			}
			continue
		}

		sess, err := c.login(ctx, creds)
		if err != nil {
			if isContextDone(err) {
				return ctx.Err()
			}
			c.health.markDisconnected("authentication failed: " + err.Error())
			if err := sleepContext(ctx, c.reconnectDelay); err != nil {
				return err
			}
			continue
		}

		if err := c.Drain(ctx); err != nil {
			if isContextDone(err) {
				return ctx.Err()
			}
			c.health.markDisconnected("drain: " + err.Error())
		}

		listener, err := c.newListener(sess)
		if err != nil {
			c.health.markDisconnected("listener init failed: " + err.Error())
			if err := sleepContext(ctx, c.reconnectDelay); err != nil {
				return err
			}
			continue
		}

		if err := listener.Start(ctx); err != nil {
			listener.Stop()
			if isContextDone(err) {
				return ctx.Err()
			}
			c.health.markDisconnected("listener start failed: " + err.Error())
			if err := sleepContext(ctx, c.reconnectDelay); err != nil {
				return err
			}
			continue
		}

		c.health.markConnected()
		if err := c.runListener(ctx, listener); err != nil {
			listener.Stop()
			if err == context.Canceled || err == context.DeadlineExceeded {
				return err
			}
			delay := c.reconnectDelay
			if isDuplicateSessionError(err) {
				c.health.markDisconnected("duplicate session; waiting to retry")
				delay = c.duplicateSessionDelay
			} else {
				c.health.markDisconnected(err.Error())
			}
			if err := sleepContext(ctx, delay); err != nil {
				return err
			}
			continue
		}
		listener.Stop()
	}
}

func isDuplicateSessionError(err error) bool {
	var disconnect *protocol.DisconnectError
	if !errors.As(err, &disconnect) {
		return false
	}
	return disconnect.Code == protocol.CloseCodeDuplicate
}

func (c *Connector) threadTypeForChat(chatID string) protocol.ThreadType {
	chatID = strings.TrimSpace(chatID)
	if c.groupPolicy.Enabled && c.groupPolicy.Allowlist[chatID] {
		return protocol.ThreadTypeGroup
	}
	return protocol.ThreadTypeUser
}

func (c *Connector) Notify(ctx context.Context, chatID string, delta model.ReplayDelta, dedupeKey string) error {
	return c.outbound.Notify(ctx, chatID, delta, dedupeKey)
}

func (c *Connector) Drain(ctx context.Context) error {
	return c.outbound.Drain(ctx)
}

func (c *Connector) SendText(ctx context.Context, chatID, text string) error {
	creds, ok, err := LoadStoredCredentials(ctx, c.outbound.db)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("zalo personal connector: not authenticated")
	}
	if c.sendText == nil {
		return fmt.Errorf("zalo personal connector: send path not configured")
	}
	for _, chunk := range splitTextChunks(text) {
		if err := c.sendText(ctx, creds, chatID, chunk); err != nil {
			return err
		}
	}
	if c.threadState != nil {
		if err := c.threadState.Upsert(ctx, threadstate.Summary{
			ConnectorID:        c.Metadata().ID,
			AccountID:          creds.AccountID,
			ThreadID:           strings.TrimSpace(chatID),
			ThreadType:         threadTypeLabel(c.threadTypeForChat(chatID)),
			LastMessagePreview: strings.TrimSpace(text),
			LastMessageAt:      time.Now().UTC(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Connector) ConnectorHealthSnapshot() model.ConnectorHealthSnapshot {
	return c.health.snapshotCopy()
}

func (c *Connector) ConfiguredConnectorHealth(
	ctx context.Context,
	current model.ConnectorHealthSnapshot,
) (model.ConnectorHealthSnapshot, bool, error) {
	_, ok, err := LoadStoredCredentials(ctx, c.outbound.db)
	if err != nil {
		return model.ConnectorHealthSnapshot{}, false, err
	}
	if !ok {
		return model.ConnectorHealthSnapshot{}, false, nil
	}
	if !shouldOverrideWithConfiguredReadiness(current) {
		return model.ConnectorHealthSnapshot{}, false, nil
	}
	return model.ConnectorHealthSnapshot{
		ConnectorID:      c.Metadata().ID,
		State:            model.ConnectorHealthUnknown,
		Summary:          "credentials stored",
		CheckedAt:        time.Now().UTC(),
		RestartSuggested: false,
	}, true, nil
}

func shouldOverrideWithConfiguredReadiness(snapshot model.ConnectorHealthSnapshot) bool {
	summary := strings.TrimSpace(strings.ToLower(snapshot.Summary))
	switch summary {
	case "", "awaiting first authentication", "awaiting first poll", "not authenticated":
		return true
	default:
		return snapshot.State == model.ConnectorHealthUnknown
	}
}

func (c *Connector) runListener(ctx context.Context, listener SessionListener) error {
	messages := listener.Messages()
	errors := listener.Errors()
	disconnected := listener.Disconnected()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-messages:
			if !ok {
				messages = nil
				if errors == nil && disconnected == nil {
					return fmt.Errorf("listener stopped")
				}
				continue
			}
			if err := c.recordThreadActivity(ctx, msg); err != nil {
				if isContextDone(err) {
					return ctx.Err()
				}
				return fmt.Errorf("listener thread activity: %w", err)
			}
			env, err := NormalizeInboundMessageWithPolicy(msg, c.groupPolicy)
			if err != nil {
				if strings.Contains(err.Error(), "DM only") ||
					strings.Contains(err.Error(), "text is required") ||
					strings.Contains(err.Error(), "group not allowed") ||
					strings.Contains(err.Error(), "group mention required") {
					continue
				}
				return fmt.Errorf("listener normalize: %w", err)
			}
			if err := c.inbound.Dispatch(ctx, env); err != nil {
				if isContextDone(err) {
					return ctx.Err()
				}
				return err
			}
		case err, ok := <-errors:
			if !ok {
				errors = nil
				if messages == nil && disconnected == nil {
					return fmt.Errorf("listener stopped")
				}
				continue
			}
			if err != nil {
				c.health.markDisconnected("listener error: " + err.Error())
			}
		case err, ok := <-disconnected:
			if !ok {
				return fmt.Errorf("listener disconnected")
			}
			if err == nil {
				return fmt.Errorf("listener disconnected")
			}
			return fmt.Errorf("listener disconnected: %w", err)
		}
	}
}

func (c *Connector) recordThreadActivity(ctx context.Context, msg IncomingMessage) error {
	if c.threadState == nil {
		return nil
	}
	return c.threadState.Upsert(ctx, threadstate.Summary{
		ConnectorID:        c.Metadata().ID,
		AccountID:          strings.TrimSpace(msg.AccountID),
		ThreadID:           strings.TrimSpace(msg.ConversationID),
		ThreadType:         incomingThreadTypeLabel(msg),
		LastMessagePreview: strings.TrimSpace(msg.Text),
		LastMessageAt:      time.Now().UTC(),
	})
}

func incomingThreadTypeLabel(msg IncomingMessage) string {
	if msg.IsDirect {
		return "contact"
	}
	return "group"
}

func threadTypeLabel(threadType protocol.ThreadType) string {
	if threadType == protocol.ThreadTypeGroup {
		return "group"
	}
	return "contact"
}

func optionalLanguage(language string) *string {
	language = strings.TrimSpace(language)
	if language == "" {
		return nil
	}
	return &language
}

func splitTextChunks(text string) []string {
	if len(text) <= maxTextChunkBytes {
		return []string{text}
	}

	chunks := make([]string, 0, len(text)/maxTextChunkBytes+1)
	for len(text) > maxTextChunkBytes {
		cutAt := safeChunkBoundary(text, maxTextChunkBytes)
		if idx := strings.LastIndex(text[:cutAt], "\n"); idx > cutAt/2 {
			cutAt = idx + 1
		}
		chunks = append(chunks, text[:cutAt])
		text = text[cutAt:]
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}

func safeChunkBoundary(text string, limit int) int {
	if limit >= len(text) {
		return len(text)
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	if limit == 0 {
		_, size := utf8.DecodeRuneInString(text)
		return size
	}
	return limit
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isContextDone(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

var _ model.Connector = (*Connector)(nil)
var _ TextSender = (*Connector)(nil)
