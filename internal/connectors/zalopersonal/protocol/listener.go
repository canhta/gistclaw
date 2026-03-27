package protocol

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	CloseCodeDuplicate = 3000
	msgBufferSize      = 64
	minEncDataLen      = 48
	stableThreshold    = 60 * time.Second
)

type DisconnectError struct {
	Code   int
	Reason string
}

func (e *DisconnectError) Error() string {
	if e == nil {
		return "zalo personal protocol: disconnected"
	}
	if e.Reason == "" {
		return fmt.Sprintf("zalo personal protocol: disconnected (%d)", e.Code)
	}
	return fmt.Sprintf("zalo personal protocol: disconnected (%d): %s", e.Code, e.Reason)
}

type retryState struct {
	count int
	max   int
	times []int
}

type wsClient interface {
	ReadMessage(ctx context.Context) ([]byte, error)
	WriteMessage(ctx context.Context, data []byte) error
	Close(code int, reason string)
}

type wsDialFunc func(ctx context.Context, wsURL string, headers http.Header, jar http.CookieJar) (wsClient, error)

type Listener struct {
	mu sync.RWMutex

	sess        *Session
	wsURLs      []string
	wsURL       string
	rotateCount int
	dialWS      wsDialFunc
	client      wsClient
	cancel      context.CancelFunc
	wg          sync.WaitGroup

	cipherKey    string
	pingCancel   context.CancelFunc
	connectedAt  time.Time
	retryStates  map[string]*retryState
	stopped      bool
	retryBackoff func(ctx context.Context, delay time.Duration) error

	messageCh      chan Message
	errorCh        chan error
	disconnectedCh chan error
}

func NewListener(sess *Session) (*Listener, error) {
	if sess == nil || sess.LoginInfo == nil || len(sess.LoginInfo.ZpwWebsocket) == 0 {
		return nil, fmt.Errorf("zalo personal protocol: no websocket URLs in session")
	}

	return &Listener{
		sess:        sess,
		wsURLs:      append([]string(nil), sess.LoginInfo.ZpwWebsocket...),
		wsURL:       buildWSURL(sess, sess.LoginInfo.ZpwWebsocket[0]),
		retryStates: buildListenerRetryStates(sess.Settings),
		dialWS: func(ctx context.Context, wsURL string, headers http.Header, jar http.CookieJar) (wsClient, error) {
			return DialWS(ctx, wsURL, headers, jar)
		},
		retryBackoff: func(ctx context.Context, delay time.Duration) error {
			return sleepContext(ctx, delay)
		},
		messageCh:      make(chan Message, msgBufferSize),
		errorCh:        make(chan error, 16),
		disconnectedCh: make(chan error, 4),
	}, nil
}

func (ln *Listener) Messages() <-chan Message { return ln.messageCh }

func (ln *Listener) Errors() <-chan error { return ln.errorCh }

func (ln *Listener) Disconnected() <-chan error { return ln.disconnectedCh }

func (ln *Listener) Start(ctx context.Context) error {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	if ln.client != nil {
		return fmt.Errorf("zalo personal protocol: listener already started")
	}

	lctx, cancel := context.WithCancel(ctx)
	ln.stopped = false
	ln.cancel = cancel
	if err := ln.connect(lctx); err != nil {
		cancel()
		ln.cancel = nil
		return err
	}
	ln.wg.Add(1)
	go ln.run(lctx)
	return nil
}

func (ln *Listener) Stop() {
	ln.mu.Lock()
	cancel := ln.cancel
	client := ln.client
	pingCancel := ln.pingCancel
	ln.stopped = true
	ln.client = nil
	ln.cancel = nil
	ln.pingCancel = nil
	ln.mu.Unlock()

	if pingCancel != nil {
		pingCancel()
	}
	if cancel != nil {
		cancel()
	}
	if client != nil {
		client.Close(1000, "")
	}
	ln.wg.Wait()
}

func (ln *Listener) run(ctx context.Context) {
	defer ln.wg.Done()

	for {
		if ctx.Err() != nil {
			return
		}

		ln.mu.RLock()
		client := ln.client
		ln.mu.RUnlock()
		if client == nil {
			return
		}

		data, err := client.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if ln.handleDisconnect(ctx, parseWSCloseError(err)) {
				continue
			}
			return
		}
		if err := ln.handleFrame(ctx, data); err != nil {
			if ln.handleDisconnect(ctx, err) {
				continue
			}
			return
		}
	}
}

func (ln *Listener) handleFrame(ctx context.Context, data []byte) error {
	if len(data) < 4 {
		return nil
	}

	version := data[0]
	cmd := binary.LittleEndian.Uint16(data[1:3])
	subCmd := data[3]
	body := data[4:]

	var envelope struct {
		Key     *string `json:"key"`
		Encrypt uint    `json:"encrypt"`
		Data    string  `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		emit(ctx, ln.errorCh, fmt.Errorf("zalo personal protocol: parse ws frame: %w", err))
		return nil
	}

	switch fmt.Sprintf("%d_%d_%d", version, cmd, subCmd) {
	case "1_1_1":
		ln.handleCipherKey(ctx, envelope.Key)
	case "1_501_0":
		ln.handleUserMessages(ctx, envelope.Data, envelope.Encrypt)
	case "1_521_0":
		ln.handleGroupMessages(ctx, envelope.Data, envelope.Encrypt)
	case "1_3000_0":
		return &DisconnectError{
			Code:   CloseCodeDuplicate,
			Reason: "duplicate session",
		}
	}
	return nil
}

func (ln *Listener) handleCipherKey(ctx context.Context, key *string) {
	if key == nil || *key == "" {
		return
	}

	ln.mu.Lock()
	ln.cipherKey = *key
	if ln.pingCancel != nil {
		ln.mu.Unlock()
		return
	}
	pingCtx, cancel := context.WithCancel(ctx)
	ln.pingCancel = cancel
	client := ln.client
	var interval time.Duration
	if ln.sess != nil && ln.sess.Settings != nil {
		interval = time.Duration(ln.sess.Settings.Features.Socket.PingInterval) * time.Millisecond
	}
	ln.mu.Unlock()

	if client == nil || interval <= 0 {
		return
	}

	go ln.pingLoop(pingCtx, interval)
}

func (ln *Listener) handleUserMessages(ctx context.Context, data string, encType uint) {
	ln.mu.RLock()
	cipherKey := ln.cipherKey
	ln.mu.RUnlock()

	payload, err := ln.decryptEventData(data, encType, cipherKey)
	if err != nil {
		emit(ctx, ln.errorCh, fmt.Errorf("zalo personal protocol: decrypt user message: %w", err))
		return
	}

	var envelope struct {
		Data struct {
			Msgs []json.RawMessage `json:"msgs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		emit(ctx, ln.errorCh, fmt.Errorf("zalo personal protocol: parse user messages: %w", err))
		return
	}

	for _, raw := range envelope.Data.Msgs {
		var msg TMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		userMsg := NewUserMessage(ln.sess.UID, msg)
		if userMsg.IsSelf() {
			continue
		}
		emit(ctx, ln.messageCh, Message(userMsg))
	}
}

func (ln *Listener) handleGroupMessages(ctx context.Context, data string, encType uint) {
	ln.mu.RLock()
	cipherKey := ln.cipherKey
	ln.mu.RUnlock()

	payload, err := ln.decryptEventData(data, encType, cipherKey)
	if err != nil {
		emit(ctx, ln.errorCh, fmt.Errorf("zalo personal protocol: decrypt group message: %w", err))
		return
	}

	var envelope struct {
		Data struct {
			GroupMsgs []json.RawMessage `json:"groupMsgs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		emit(ctx, ln.errorCh, fmt.Errorf("zalo personal protocol: parse group messages: %w", err))
		return
	}

	for _, raw := range envelope.Data.GroupMsgs {
		var msg TGroupMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		groupMsg := NewGroupMessage(ln.sess.UID, msg)
		if groupMsg.IsSelf() {
			continue
		}
		emit(ctx, ln.messageCh, Message(groupMsg))
	}
}

func (ln *Listener) decryptEventData(data string, encType uint, cipherKey string) ([]byte, error) {
	var result []byte
	var err error

	switch encType {
	case 0:
		result = []byte(data)
	case 1:
		result, err = base64.StdEncoding.DecodeString(data)
	case 2:
		var raw []byte
		raw, err = ln.decryptAESGCMPayload(data, cipherKey)
		if err == nil {
			result, err = decompressGzip(raw)
		}
	case 3:
		result, err = ln.decryptAESGCMPayload(data, cipherKey)
	default:
		return nil, fmt.Errorf("unknown encryption type %d", encType)
	}
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(result) {
		return nil, fmt.Errorf("zalo personal protocol: decrypted payload is not valid UTF-8")
	}
	return result, nil
}

func (ln *Listener) decryptAESGCMPayload(data, cipherKey string) ([]byte, error) {
	if cipherKey == "" {
		return nil, fmt.Errorf("zalo personal protocol: cipher key required")
	}

	unescaped, err := url.PathUnescape(data)
	if err != nil {
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(unescaped)
	if err != nil {
		return nil, err
	}
	if len(decoded) < minEncDataLen {
		return nil, fmt.Errorf("zalo personal protocol: encrypted data too short (%d bytes)", len(decoded))
	}

	key, err := base64.StdEncoding.DecodeString(cipherKey)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: decode cipher key: %w", err)
	}

	iv := decoded[0:16]
	aad := decoded[16:32]
	ct := decoded[32:]
	return DecodeAESGCM(key, iv, aad, ct)
}

func decompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: gzip reader: %w", err)
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (ln *Listener) pingLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ln.sendPing(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ln.sendPing(ctx)
		}
	}
}

func (ln *Listener) sendPing(ctx context.Context) {
	body, _ := json.Marshal(map[string]any{"eventId": time.Now().UnixMilli()})
	frame := make([]byte, 4+len(body))
	frame[0] = 1
	binary.LittleEndian.PutUint16(frame[1:3], 2)
	frame[3] = 1
	copy(frame[4:], body)

	ln.mu.RLock()
	client := ln.client
	ln.mu.RUnlock()
	if client != nil {
		_ = client.WriteMessage(ctx, frame)
	}
}

func buildWSURL(sess *Session, base string) string {
	return makeURL(sess, base, map[string]any{"t": time.Now().UnixMilli()}, true)
}

func (ln *Listener) connect(ctx context.Context) error {
	headers := http.Header{}
	headers.Set("Accept-Language", "en-US,en;q=0.9")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Origin", DefaultBaseURL.String())
	headers.Set("Pragma", "no-cache")
	headers.Set("User-Agent", ln.sess.UserAgent)

	client, err := ln.dialWS(ctx, ln.wsURL, headers, ln.sess.CookieJar)
	if err != nil {
		return err
	}

	ln.client = client
	ln.connectedAt = time.Now().UTC()
	return nil
}

func (ln *Listener) handleDisconnect(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}

	disconnect, ok := err.(*DisconnectError)
	if !ok {
		disconnect = &DisconnectError{Code: 1006, Reason: err.Error()}
	}

	wasStable := !ln.connectedAt.IsZero() && time.Since(ln.connectedAt) > stableThreshold
	ln.resetAfterDisconnect()
	emit(ctx, ln.errorCh, error(disconnect))

	if disconnect.Code == CloseCodeDuplicate {
		emit(ctx, ln.disconnectedCh, error(disconnect))
		return false
	}

	if wasStable {
		ln.resetRetryCounters()
	}

	delay, ok := ln.canRetry(disconnect.Code)
	if !ok {
		emit(ctx, ln.disconnectedCh, error(disconnect))
		return false
	}

	ln.tryRotateEndpoint(disconnect.Code)
	if err := ln.retryBackoff(ctx, time.Duration(delay)*time.Millisecond); err != nil {
		return false
	}
	if err := ln.connect(ctx); err != nil {
		emit(ctx, ln.disconnectedCh, error(parseWSCloseError(err)))
		return false
	}
	return true
}

func (ln *Listener) resetAfterDisconnect() {
	ln.mu.Lock()
	defer ln.mu.Unlock()
	if ln.pingCancel != nil {
		ln.pingCancel()
		ln.pingCancel = nil
	}
	if ln.client != nil {
		ln.client.Close(1000, "")
		ln.client = nil
	}
	ln.cipherKey = ""
	ln.connectedAt = time.Time{}
}

func (ln *Listener) resetRetryCounters() {
	ln.mu.Lock()
	defer ln.mu.Unlock()
	for _, state := range ln.retryStates {
		state.count = 0
	}
	ln.rotateCount = 0
	if len(ln.wsURLs) > 0 {
		ln.wsURL = buildWSURL(ln.sess, ln.wsURLs[0])
	}
}

func (ln *Listener) canRetry(code int) (int, bool) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	state, ok := ln.retryStates[fmt.Sprint(code)]
	if !ok || state == nil || state.max == 0 || len(state.times) == 0 {
		return 0, false
	}
	if state.count >= state.max {
		return 0, false
	}

	index := state.count
	state.count++
	delay := state.times[len(state.times)-1]
	if index < len(state.times) {
		delay = state.times[index]
	}
	return delay, true
}

func (ln *Listener) tryRotateEndpoint(code int) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	if ln.sess == nil || ln.sess.Settings == nil {
		return
	}
	if ln.rotateCount >= len(ln.wsURLs)-1 {
		return
	}
	for _, candidate := range ln.sess.Settings.Features.Socket.RotateErrorCodes {
		if candidate != code {
			continue
		}
		ln.rotateCount++
		ln.wsURL = buildWSURL(ln.sess, ln.wsURLs[ln.rotateCount])
		return
	}
}

func buildListenerRetryStates(settings *Settings) map[string]*retryState {
	states := make(map[string]*retryState, 8)
	if settings == nil {
		return states
	}
	for reason, cfg := range settings.Features.Socket.Retries {
		maxRetries := 0
		if cfg.Max != nil {
			maxRetries = *cfg.Max
		}
		if len(cfg.Times) == 0 {
			continue
		}
		states[reason] = &retryState{max: maxRetries, times: append([]int(nil), cfg.Times...)}
	}
	return states
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func emit[T any](ctx context.Context, ch chan T, value T) {
	select {
	case <-ctx.Done():
		return
	case ch <- value:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- value:
		default:
		}
	}
}
