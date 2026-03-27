package protocol

import (
	"context"
	"errors"
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

	base := &url.URL{Scheme: "https", Host: "chat.zalo.me"}
	cookies := sess.CookieJar.Cookies(base)
	if len(cookies) != 2 {
		t.Fatalf("expected 2 cookies in jar, got %d", len(cookies))
	}
	if cookies[0].Name != "zpw_sek" && cookies[1].Name != "zpw_sek" {
		t.Fatalf("expected zpw_sek cookie in jar, got %+v", cookies)
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
