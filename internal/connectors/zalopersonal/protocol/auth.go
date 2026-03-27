package protocol

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var ErrQRLoginNotImplemented = errors.New("zalo personal protocol: qr login not implemented")

func LoginQR(ctx context.Context, qrCallback func([]byte)) (*Credentials, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, ErrQRLoginNotImplemented
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
