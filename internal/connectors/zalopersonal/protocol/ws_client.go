package protocol

import (
	"net/http"
	"net/url"
	"strings"
)

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
