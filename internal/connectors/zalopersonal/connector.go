package zalopersonal

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	login                  func(ctx context.Context, creds StoredCredentials) (*listenerSession, error)
	newListener            func(sess *listenerSession) (SessionListener, error)
	credentialPollInterval time.Duration
	reconnectDelay         time.Duration
}

func NewConnector(db *store.DB, cs *conversations.ConversationStore, rt ConnectorRuntime, defaultAgentID string) *Connector {
	connector := &Connector{
		inbound:                NewInboundDispatcher(rt, defaultAgentID),
		health:                 NewHealthState(),
		credentialPollInterval: 5 * time.Second,
		reconnectDelay:         2 * time.Second,
	}
	connector.outbound = NewOutboundDispatcher(connector, db, cs)
	connector.login = func(ctx context.Context, creds StoredCredentials) (*listenerSession, error) {
		_, err := protocol.LoginWithCredentials(ctx, protocol.Credentials{
			IMEI:      creds.IMEI,
			Cookie:    creds.Cookie,
			UserAgent: creds.UserAgent,
			Language:  optionalLanguage(creds.Language),
		})
		if err != nil {
			return nil, err
		}
		return &listenerSession{AccountID: creds.AccountID, Language: creds.Language}, nil
	}
	connector.newListener = func(*listenerSession) (SessionListener, error) {
		return nil, fmt.Errorf("zalo personal connector: live listener not implemented")
	}
	return connector
}

func (c *Connector) ID() string {
	return "zalo_personal"
}

func (c *Connector) Start(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := c.Drain(ctx); err != nil {
			if isContextDone(err) {
				return ctx.Err()
			}
			c.health.markDisconnected("drain: " + err.Error())
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
			c.health.markDisconnected(err.Error())
			if err := sleepContext(ctx, c.reconnectDelay); err != nil {
				return err
			}
			continue
		}
		listener.Stop()
	}
}

func (c *Connector) Notify(ctx context.Context, chatID string, delta model.ReplayDelta, dedupeKey string) error {
	return c.outbound.Notify(ctx, chatID, delta, dedupeKey)
}

func (c *Connector) Drain(ctx context.Context) error {
	return c.outbound.Drain(ctx)
}

func (c *Connector) SendText(_ context.Context, _ string, _ string) error {
	return fmt.Errorf("zalo personal connector: live send not implemented")
}

func (c *Connector) ConnectorHealthSnapshot() model.ConnectorHealthSnapshot {
	return c.health.snapshotCopy()
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
			env, err := NormalizeInboundMessage(msg)
			if err != nil {
				if strings.Contains(err.Error(), "DM only") || strings.Contains(err.Error(), "text is required") {
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

func optionalLanguage(language string) *string {
	language = strings.TrimSpace(language)
	if language == "" {
		return nil
	}
	return &language
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
	return err == context.Canceled || err == context.DeadlineExceeded
}

var _ model.Connector = (*Connector)(nil)
var _ TextSender = (*Connector)(nil)
