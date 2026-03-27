package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

func TestLoginWithCredentialsRejectsInvalidCredentials(t *testing.T) {
	t.Parallel()

	_, err := LoginWithCredentials(context.Background(), Credentials{})
	if err == nil {
		t.Fatal("expected invalid credentials error")
	}
}

func TestLoginWithCredentialsSeedsCookieJar(t *testing.T) {
	t.Parallel()

	oldTransport := defaultHTTPTransport
	defaultHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "wpa.chat.zalo.me" && req.URL.Path == "/api/login/getLoginInfo":
			key, err := deriveEncryptKey(req.URL.Query().Get("zcid_ext"), req.URL.Query().Get("zcid"))
			if err != nil {
				t.Fatalf("deriveEncryptKey: %v", err)
			}

			payload, err := json.Marshal(Response[*LoginInfo]{
				Data: &LoginInfo{
					UID:          "uid-123",
					ZPWEnk:       "sek-123",
					ZpwWebsocket: []string{"wss://ws.chat.zalo.me/socket"},
					ZpwServiceMapV3: ZpwServiceMapV3{
						GroupPoll: []string{"https://tt-group-poll-wpa.chat.zalo.me/api"},
					},
				},
			})
			if err != nil {
				t.Fatalf("Marshal login payload: %v", err)
			}
			encrypted, err := EncodeAESCBC([]byte(key), string(payload), false)
			if err != nil {
				t.Fatalf("EncodeAESCBC: %v", err)
			}
			escaped := url.PathEscape(encrypted)
			return jsonHTTPResponse(t, Response[*string]{Data: &escaped}), nil
		case req.URL.Host == "wpa.chat.zalo.me" && req.URL.Path == "/api/login/getServerInfo":
			return rawHTTPResponse(t, `{"data":{"setttings":{"features":{"socket":{"ping_interval":5,"retries":{"main":{"times":[1,2]}}}}}}}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})
	t.Cleanup(func() { defaultHTTPTransport = oldTransport })

	lang := "vi"
	sess, err := LoginWithCredentials(context.Background(), Credentials{
		IMEI:      "imei-123",
		Cookie:    "zpw_sek=abc123; _ga=test",
		UserAgent: "Mozilla/5.0",
		Language:  &lang,
	})
	if err != nil {
		t.Fatalf("LoginWithCredentials: %v", err)
	}
	if sess.Language != lang {
		t.Fatalf("expected language %q, got %q", lang, sess.Language)
	}
	if sess.UID != "uid-123" {
		t.Fatalf("expected uid-123, got %q", sess.UID)
	}
	if sess.SecretKey != "sek-123" {
		t.Fatalf("expected secret key sek-123, got %q", sess.SecretKey)
	}
	if sess.LoginInfo == nil || sess.Settings == nil {
		t.Fatalf("expected hydrated session metadata, got %+v", sess)
	}

	base := &url.URL{Scheme: "https", Host: "chat.zalo.me"}
	cookies := sess.CookieJar.Cookies(base)
	if len(cookies) != 2 {
		t.Fatalf("expected 2 cookies in jar, got %d", len(cookies))
	}
	if cookies[0].Name != "zpw_sek" && cookies[1].Name != "zpw_sek" {
		t.Fatalf("expected zpw_sek cookie in jar, got %+v", cookies)
	}

	groupPoll := &url.URL{Scheme: "https", Host: "tt-group-poll-wpa.chat.zalo.me"}
	groupCookies := sess.CookieJar.Cookies(groupPoll)
	if len(groupCookies) == 0 {
		t.Fatalf("expected service-map cookies on group poll host, got %+v", groupCookies)
	}
}

func TestLoginQRReturnsNotImplemented(t *testing.T) {
	t.Parallel()

	_, err := LoginQR(context.Background(), nil)
	if !errors.Is(err, ErrQRLoginNotImplemented) {
		t.Fatalf("expected ErrQRLoginNotImplemented, got %v", err)
	}
}

func TestInjectCookiesMergesBaseAndHostCookies(t *testing.T) {
	t.Parallel()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	base := &url.URL{Scheme: "https", Host: "chat.zalo.me"}
	host := &url.URL{Scheme: "https", Host: "ws.chat.zalo.me"}
	jar.SetCookies(base, []*http.Cookie{{Name: "zpw_sek", Value: "base"}})
	jar.SetCookies(host, []*http.Cookie{{Name: "ws_token", Value: "host"}})

	headers := http.Header{}
	InjectCookies(headers, jar, "wss://ws.chat.zalo.me/socket")

	got := headers.Get("Cookie")
	if got == "" {
		t.Fatal("expected cookie header to be populated")
	}
	for _, want := range []string{"zpw_sek=base", "ws_token=host"} {
		if !containsCookie(got, want) {
			t.Fatalf("expected cookie header %q to contain %q", got, want)
		}
	}
}

func containsCookie(header, want string) bool {
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if part == want {
			return true
		}
	}
	return false
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonHTTPResponse(t *testing.T, payload any) *http.Response {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal JSON payload: %v", err)
	}
	return rawHTTPResponse(t, string(data))
}

func rawHTTPResponse(t *testing.T, body string) *http.Response {
	t.Helper()

	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(bytes.NewBufferString(body)),
	}
}
