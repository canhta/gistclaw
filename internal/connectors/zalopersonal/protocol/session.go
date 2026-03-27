package protocol

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0"
	DefaultLanguage     = "vi"
	DefaultAPIType      = 30
	DefaultAPIVersion   = 665
	DefaultComputerName = "Web"
	DefaultEncryptVer   = "v2"
	DefaultZCIDKey      = "3FC4F0D2AB50057BCE0D90D9187A22B1"
	MaxRedirects        = 10
)

var DefaultBaseURL = url.URL{Scheme: "https", Host: "chat.zalo.me"}
var defaultHTTPTransport http.RoundTripper = http.DefaultTransport

type Credentials struct {
	IMEI      string  `json:"imei"`
	Cookie    string  `json:"cookie"`
	UserAgent string  `json:"user_agent"`
	Language  *string `json:"language,omitempty"`
}

func (c Credentials) IsValid() bool {
	return strings.TrimSpace(c.IMEI) != "" &&
		strings.TrimSpace(c.Cookie) != "" &&
		strings.TrimSpace(c.UserAgent) != ""
}

type Session struct {
	UID       string
	IMEI      string
	UserAgent string
	Language  string
	SecretKey string

	Cookie    string
	LoginInfo *LoginInfo
	Settings  *Settings
	CookieJar http.CookieJar
	Client    *http.Client
}

func NewSession() *Session {
	jar, _ := cookiejar.New(nil)
	return &Session{
		UserAgent: DefaultUserAgent,
		Language:  DefaultLanguage,
		CookieJar: jar,
		Client: &http.Client{
			Jar:       jar,
			Transport: defaultHTTPTransport,
			Timeout:   60 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= MaxRedirects {
					return fmt.Errorf("zalo personal protocol: too many redirects")
				}
				return nil
			},
		},
	}
}

func GenerateIMEI(userAgent string) string {
	u := uuid.New().String()
	hash := md5.Sum([]byte(userAgent))
	return u + "-" + hex.EncodeToString(hash[:])
}

func LoginWithCredentials(ctx context.Context, creds Credentials) (*Session, error) {
	if !creds.IsValid() {
		return nil, fmt.Errorf("zalo personal protocol: invalid credentials")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	sess := NewSession()
	sess.IMEI = strings.TrimSpace(creds.IMEI)
	sess.UserAgent = strings.TrimSpace(creds.UserAgent)
	sess.Cookie = strings.TrimSpace(creds.Cookie)
	if creds.Language != nil && strings.TrimSpace(*creds.Language) != "" {
		sess.Language = strings.TrimSpace(*creds.Language)
	}
	BuildCookieJarFromHeader(sess.CookieJar, &DefaultBaseURL, sess.Cookie)

	loginInfo, err := fetchLoginInfo(ctx, sess)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: login: %w", err)
	}
	serverInfo, err := fetchServerInfo(ctx, sess)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: server info: %w", err)
	}
	if loginInfo == nil || serverInfo == nil || serverInfo.Settings == nil {
		return nil, fmt.Errorf("zalo personal protocol: login failed (empty response)")
	}

	sess.UID = loginInfo.UID
	sess.SecretKey = loginInfo.ZPWEnk
	sess.LoginInfo = loginInfo
	sess.Settings = serverInfo.Settings
	seedServiceMapCookies(sess, sess.Cookie)
	return sess, nil
}

func BuildCookieJarFromHeader(jar http.CookieJar, baseURL *url.URL, cookieHeader string) {
	if jar == nil || baseURL == nil {
		return
	}
	cookies := parseCookieHeader(cookieHeader, baseURL.Host)
	if len(cookies) == 0 {
		return
	}
	jar.SetCookies(baseURL, cookies)
	wpaURL := &url.URL{Scheme: "https", Host: "wpa.chat.zalo.me"}
	jar.SetCookies(wpaURL, cookies)
}

func parseCookieHeader(cookieHeader, host string) []*http.Cookie {
	parts := strings.Split(cookieHeader, ";")
	cookies := make([]*http.Cookie, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" {
			continue
		}
		cookies = append(cookies, &http.Cookie{
			Name:   name,
			Value:  value,
			Path:   "/",
			Domain: host,
		})
	}
	return cookies
}

func seedServiceMapCookies(sess *Session, cookieHeader string) {
	if sess == nil || sess.LoginInfo == nil || sess.CookieJar == nil {
		return
	}

	serviceURLs := make([]string, 0, 16)
	serviceURLs = append(serviceURLs, sess.LoginInfo.ZpwServiceMapV3.Chat...)
	serviceURLs = append(serviceURLs, sess.LoginInfo.ZpwServiceMapV3.Group...)
	serviceURLs = append(serviceURLs, sess.LoginInfo.ZpwServiceMapV3.File...)
	serviceURLs = append(serviceURLs, sess.LoginInfo.ZpwServiceMapV3.Profile...)
	serviceURLs = append(serviceURLs, sess.LoginInfo.ZpwServiceMapV3.GroupPoll...)

	seen := make(map[string]struct{}, len(serviceURLs))
	for _, rawURL := range serviceURLs {
		if strings.TrimSpace(rawURL) == "" {
			continue
		}
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Host == "" {
			continue
		}
		hostURL := &url.URL{Scheme: parsed.Scheme, Host: parsed.Host}
		key := hostURL.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		BuildCookieJarFromHeader(sess.CookieJar, hostURL, cookieHeader)
	}
}
