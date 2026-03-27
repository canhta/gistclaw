package zalopersonal

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/canhta/gistclaw/internal/connectors/zalopersonal/protocol"
)

type protocolSessionListener interface {
	Start(ctx context.Context) error
	Stop()
	Messages() <-chan protocol.Message
	Errors() <-chan error
	Disconnected() <-chan error
}

type protocolListenerAdapter struct {
	accountID string
	language  string
	listener  protocolSessionListener

	cancel       context.CancelFunc
	startOnce    sync.Once
	stopOnce     sync.Once
	wg           sync.WaitGroup
	messageCh    chan IncomingMessage
	errorCh      chan error
	disconnectCh chan error
}

var newProtocolSessionListener = func(sess *listenerSession) (SessionListener, error) {
	if sess == nil || sess.Protocol == nil {
		return nil, fmt.Errorf("zalo personal connector: protocol session is required")
	}
	listener, err := protocol.NewListener(sess.Protocol)
	if err != nil {
		return nil, err
	}
	return &protocolListenerAdapter{
		accountID:    sess.AccountID,
		language:     sess.Language,
		listener:     listener,
		messageCh:    make(chan IncomingMessage, 64),
		errorCh:      make(chan error, 16),
		disconnectCh: make(chan error, 4),
	}, nil
}

func (a *protocolListenerAdapter) Start(ctx context.Context) error {
	if err := a.listener.Start(ctx); err != nil {
		return err
	}
	a.startOnce.Do(func() {
		pumpCtx, cancel := context.WithCancel(ctx)
		a.cancel = cancel
		a.wg.Add(1)
		go a.pump(pumpCtx)
	})
	return nil
}

func (a *protocolListenerAdapter) Stop() {
	a.stopOnce.Do(func() {
		if a.cancel != nil {
			a.cancel()
		}
		a.listener.Stop()
		a.wg.Wait()
	})
}

func (a *protocolListenerAdapter) Messages() <-chan IncomingMessage { return a.messageCh }

func (a *protocolListenerAdapter) Errors() <-chan error { return a.errorCh }

func (a *protocolListenerAdapter) Disconnected() <-chan error { return a.disconnectCh }

func (a *protocolListenerAdapter) pump(ctx context.Context) {
	defer a.wg.Done()

	messages := a.listener.Messages()
	errors := a.listener.Errors()
	disconnected := a.listener.Disconnected()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-messages:
			if !ok {
				messages = nil
				if errors == nil && disconnected == nil {
					return
				}
				continue
			}
			incoming, keep := incomingMessageFromProtocolMessage(a.accountID, a.language, msg)
			if !keep {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case a.messageCh <- incoming:
			}
		case err, ok := <-errors:
			if !ok {
				errors = nil
				if messages == nil && disconnected == nil {
					return
				}
				continue
			}
			select {
			case <-ctx.Done():
				return
			case a.errorCh <- err:
			default:
			}
		case err, ok := <-disconnected:
			if !ok {
				disconnected = nil
				if messages == nil && errors == nil {
					return
				}
				continue
			}
			select {
			case <-ctx.Done():
				return
			case a.disconnectCh <- err:
			default:
			}
		}
	}
}

func incomingMessageFromProtocolMessage(accountID, language string, msg protocol.Message) (IncomingMessage, bool) {
	if msg == nil || msg.IsSelf() {
		return IncomingMessage{}, false
	}

	text := strings.TrimSpace(msg.Text())
	switch typed := msg.(type) {
	case protocol.UserMessage:
		if text == "" {
			return IncomingMessage{}, false
		}
		return IncomingMessage{
			AccountID:      strings.TrimSpace(accountID),
			SenderID:       strings.TrimSpace(msg.SenderID()),
			ConversationID: strings.TrimSpace(msg.ThreadID()),
			MessageID:      strings.TrimSpace(msg.MessageID()),
			Text:           text,
			LanguageHint:   strings.TrimSpace(language),
			IsDirect:       true,
		}, true
	case protocol.GroupMessage:
		if text == "" {
			return IncomingMessage{}, false
		}
		return IncomingMessage{
			AccountID:      strings.TrimSpace(accountID),
			SenderID:       strings.TrimSpace(msg.SenderID()),
			ConversationID: strings.TrimSpace(msg.ThreadID()),
			MessageID:      strings.TrimSpace(msg.MessageID()),
			Text:           text,
			LanguageHint:   strings.TrimSpace(language),
			IsDirect:       false,
			Mentioned:      typed.MentionsAccount(accountID),
		}, true
	default:
		return IncomingMessage{}, false
	}
}
