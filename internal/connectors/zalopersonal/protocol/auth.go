package protocol

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var ErrQRLoginNotImplemented = errors.New("zalo personal protocol: qr login not implemented")

func LoginQR(ctx context.Context, qrCallback func([]byte)) (*Credentials, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	sess := NewSession()
	ver, err := loadLoginPage(ctx, sess)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: load login page: %w", err)
	}

	qrGetLoginInfo(ctx, sess, ver)
	qrVerifyClient(ctx, sess, ver)

	qrData, imgBytes, err := qrGenerateCode(ctx, sess, ver)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: generate qr: %w", err)
	}
	if qrCallback != nil {
		qrCallback(imgBytes)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()

	if err := qrWaitingScan(timeoutCtx, sess, ver, qrData.Code); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: waiting scan: %w", err)
	}
	if err := qrWaitingConfirm(timeoutCtx, sess, ver, qrData.Code); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: waiting confirm: %w", err)
	}
	if err := qrCheckSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: check session: %w", err)
	}
	userInfo, err := qrGetUserInfo(ctx, sess)
	if err != nil || userInfo == nil || !userInfo.Logged {
		return nil, fmt.Errorf("zalo personal protocol: get user info failed or not logged in")
	}

	imei := GenerateIMEI(sess.UserAgent)
	lang := sess.Language
	cookie := cookieHeader(sess.CookieJar.Cookies(&DefaultBaseURL))
	if strings.TrimSpace(cookie) == "" {
		return nil, fmt.Errorf("zalo personal protocol: qr login completed without chat cookies")
	}

	return &Credentials{
		IMEI:      imei,
		Cookie:    cookie,
		UserAgent: sess.UserAgent,
		Language:  &lang,
	}, nil
}

func fetchLoginInfo(ctx context.Context, sess *Session) (*LoginInfo, error) {
	params, enk, err := getEncryptParam(sess, "getlogininfo")
	if err != nil {
		return nil, err
	}

	params["nretry"] = 0
	loginURL := makeURL(sess, "https://wpa.chat.zalo.me/api/login/getLoginInfo", params, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loginURL, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var base Response[*string]
	if err := readJSON(resp, &base); err != nil {
		return nil, fmt.Errorf("parse login response: %w", err)
	}
	if enk == nil || base.Data == nil {
		return nil, fmt.Errorf("no encrypted data in response")
	}

	unescaped, err := url.PathUnescape(*base.Data)
	if err != nil {
		return nil, err
	}
	plain, err := DecodeAESCBC([]byte(*enk), unescaped)
	if err != nil {
		return nil, fmt.Errorf("decrypt login data: %w", err)
	}

	var result Response[*LoginInfo]
	if err := json.Unmarshal(plain, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func fetchServerInfo(ctx context.Context, sess *Session) (*ServerInfo, error) {
	params, _, err := getEncryptParam(sess, "getserverinfo")
	if err != nil {
		return nil, err
	}

	signkey, _ := params["signkey"].(string)
	serverURL := makeURL(sess, "https://wpa.chat.zalo.me/api/login/getServerInfo", map[string]any{
		"signkey":        signkey,
		"imei":           sess.IMEI,
		"type":           DefaultAPIType,
		"client_version": DefaultAPIVersion,
		"computer_name":  DefaultComputerName,
	}, false)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data *ServerInfo `json:"data"`
	}
	if err := readJSON(resp, &result); err != nil {
		return nil, fmt.Errorf("parse server info: %w", err)
	}
	return result.Data, nil
}

func setDefaultHeaders(req *http.Request, sess *Session) {
	for key, values := range defaultHeaders(sess) {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
}

func readJSON(resp *http.Response, target any) error {
	var reader io.ReadCloser
	var err error

	switch strings.ToLower(resp.Header.Get("Content-Encoding")) {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return err
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}

	return json.NewDecoder(reader).Decode(target)
}

func loadLoginPage(ctx context.Context, sess *Session) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://id.zalo.me/account?continue=https%3A%2F%2Fchat.zalo.me%2F", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", sess.UserAgent)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	match := regexp.MustCompile(`https:\/\/stc-zlogin\.zdn\.vn\/main-([\d.]+)\.js`).FindSubmatch(body)
	if len(match) < 2 {
		return "", fmt.Errorf("zalo personal protocol: version not found in login page html")
	}
	return string(match[1]), nil
}

var qrHeaders = http.Header{
	"Accept":          {"*/*"},
	"Content-Type":    {"application/x-www-form-urlencoded"},
	"Sec-Fetch-Dest":  {"empty"},
	"Sec-Fetch-Mode":  {"cors"},
	"Sec-Fetch-Site":  {"same-origin"},
	"Referer":         {"https://id.zalo.me/account?continue=https%3A%2F%2Fzalo.me%2Fpc"},
	"Referrer-Policy": {"strict-origin-when-cross-origin"},
}

func qrPost(ctx context.Context, sess *Session, endpoint string, formData map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, buildFormBody(formData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", sess.UserAgent)
	for key, values := range qrHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return sess.Client.Do(req)
}

func qrGetLoginInfo(ctx context.Context, sess *Session, ver string) {
	resp, err := qrPost(ctx, sess, "https://id.zalo.me/account/logininfo", map[string]string{
		"v": ver, "continue": "https://zalo.me/pc",
	})
	if err == nil {
		resp.Body.Close()
	}
}

func qrVerifyClient(ctx context.Context, sess *Session, ver string) {
	resp, err := qrPost(ctx, sess, "https://id.zalo.me/account/verify-client", map[string]string{
		"v": ver, "type": "device", "continue": "https://zalo.me/pc",
	})
	if err == nil {
		resp.Body.Close()
	}
}

func qrGenerateCode(ctx context.Context, sess *Session, ver string) (*QRGeneratedData, []byte, error) {
	resp, err := qrPost(ctx, sess, "https://id.zalo.me/account/authen/qr/generate", map[string]string{
		"v": ver, "continue": "https://zalo.me/pc",
	})
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	var body Response[QRGeneratedData]
	if err := readJSON(resp, &body); err != nil {
		return nil, nil, err
	}

	b64 := strings.TrimPrefix(body.Data.Image, "data:image/png;base64,")
	imgBytes, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, nil, err
	}
	return &body.Data, imgBytes, nil
}

func qrWaitingScan(ctx context.Context, sess *Session, ver, code string) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		resp, err := qrPost(ctx, sess, "https://id.zalo.me/account/authen/qr/waiting-scan", map[string]string{
			"v": ver, "code": code, "continue": "https://zalo.me/pc",
		})
		if err != nil {
			return err
		}

		var body Response[QRScannedData]
		if err := readJSON(resp, &body); err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()

		if body.ErrorCode == 8 {
			continue
		}
		if body.ErrorCode != 0 {
			return fmt.Errorf("zalo personal protocol: scan error code %d: %s", body.ErrorCode, body.ErrorMessage)
		}
		return nil
	}
}

func qrWaitingConfirm(ctx context.Context, sess *Session, ver, code string) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		resp, err := qrPost(ctx, sess, "https://id.zalo.me/account/authen/qr/waiting-confirm", map[string]string{
			"v": ver, "code": code, "gToken": "", "gAction": "CONFIRM_QR", "continue": "https://zalo.me/pc",
		})
		if err != nil {
			return err
		}

		var body Response[struct{}]
		if err := readJSON(resp, &body); err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()

		if body.ErrorCode == 8 {
			continue
		}
		if body.ErrorCode == -13 {
			return fmt.Errorf("zalo personal protocol: qr login declined by user")
		}
		if body.ErrorCode != 0 {
			return fmt.Errorf("zalo personal protocol: confirm error code %d: %s", body.ErrorCode, body.ErrorMessage)
		}
		return nil
	}
}

func qrCheckSession(ctx context.Context, sess *Session) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://id.zalo.me/account/checksession?continue=https%3A%2F%2Fchat.zalo.me%2Findex.html", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", sess.UserAgent)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func qrGetUserInfo(ctx context.Context, sess *Session) (*QRUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://jr.chat.zalo.me/jr/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", sess.UserAgent)
	req.Header.Set("Referer", DefaultBaseURL.String()+"/")

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body Response[QRUserInfo]
	if err := readJSON(resp, &body); err != nil {
		return nil, err
	}
	return &body.Data, nil
}

func cookieHeader(cookies []*http.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}
