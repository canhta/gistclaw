package protocol

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type WSClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func DialWS(ctx context.Context, wsURL string, headers http.Header, jar http.CookieJar) (*WSClient, error) {
	dialer := websocket.Dialer{
		EnableCompression: true,
	}
	if jar != nil {
		InjectCookies(headers, jar, wsURL)
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: ws dial: %w", err)
	}
	conn.SetReadLimit(1 << 20)
	return &WSClient{conn: conn}, nil
}

func InjectCookies(headers http.Header, jar http.CookieJar, wsURL string) {
	if headers == nil || jar == nil {
		return
	}

	seen := make(map[string]string, 4)
	baseURL := &url.URL{Scheme: "https", Host: "chat.zalo.me", Path: "/"}
	for _, cookie := range jar.Cookies(baseURL) {
		seen[cookie.Name] = cookie.Name + "=" + cookie.Value
	}
	if parsed, err := url.Parse(strings.Replace(wsURL, "wss://", "https://", 1)); err == nil {
		for _, cookie := range jar.Cookies(parsed) {
			seen[cookie.Name] = cookie.Name + "=" + cookie.Value
		}
	}
	if len(seen) == 0 {
		return
	}

	parts := make([]string, 0, len(seen))
	for _, cookie := range seen {
		parts = append(parts, cookie)
	}
	cookieHeader := strings.Join(parts, "; ")
	if existing := headers.Get("Cookie"); existing != "" {
		cookieHeader = existing + "; " + cookieHeader
	}
	headers.Set("Cookie", cookieHeader)
}

func (c *WSClient) ReadMessage(context.Context) ([]byte, error) {
	_, data, err := c.conn.ReadMessage()
	return data, err
}

func (c *WSClient) WriteMessage(_ context.Context, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *WSClient) Close(code int, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason))
	_ = c.conn.Close()
}

func parseWSCloseError(err error) *DisconnectError {
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		return &DisconnectError{Code: closeErr.Code, Reason: closeErr.Text}
	}
	if err == nil {
		return nil
	}
	return &DisconnectError{Code: 1006, Reason: err.Error()}
}
