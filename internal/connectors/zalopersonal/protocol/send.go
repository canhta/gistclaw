package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ThreadType uint8

const (
	ThreadTypeUser ThreadType = iota
	ThreadTypeGroup
)

func SendMessage(ctx context.Context, sess *Session, threadID string, threadType ThreadType, text string) (string, error) {
	if sess == nil {
		return "", fmt.Errorf("zalo personal protocol: session is required")
	}
	if strings.TrimSpace(threadID) == "" {
		return "", fmt.Errorf("zalo personal protocol: thread ID is required")
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("zalo personal protocol: message text is required")
	}

	serviceKey := "chat"
	apiPath := "/api/message/sms"
	if threadType == ThreadTypeGroup {
		serviceKey = "group"
		apiPath = "/api/group/sendmsg"
	}

	baseURL := getServiceURL(sess, serviceKey)
	if baseURL == "" {
		return "", fmt.Errorf("zalo personal protocol: no service URL for %s", serviceKey)
	}

	payload := map[string]any{
		"message":  text,
		"clientId": time.Now().UnixMilli(),
		"ttl":      0,
	}
	if threadType == ThreadTypeGroup {
		payload["grid"] = threadID
		payload["visibility"] = 0
	} else {
		payload["toid"] = threadID
		payload["imei"] = sess.IMEI
	}

	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: encrypt send payload: %w", err)
	}

	sendURL := makeURL(sess, baseURL+apiPath, map[string]any{"nretry": 0}, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, buildFormBody(map[string]string{"params": encData}))
	if err != nil {
		return "", err
	}
	setDefaultHeaders(req, sess)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := sess.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: send message: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return "", fmt.Errorf("zalo personal protocol: parse send response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return "", fmt.Errorf("zalo personal protocol: send error code %d: %s", envelope.ErrorCode, envelope.ErrorMessage)
	}
	if envelope.Data == nil {
		return "", nil
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return "", fmt.Errorf("zalo personal protocol: decrypt send response: %w", err)
	}

	var result struct {
		MsgID json.RawMessage `json:"msgId"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		return "", fmt.Errorf("zalo personal protocol: parse send result: %w", err)
	}

	var msgID string
	if err := json.Unmarshal(result.MsgID, &msgID); err == nil {
		return msgID, nil
	}

	var numericID json.Number
	if err := json.Unmarshal(result.MsgID, &numericID); err == nil {
		return numericID.String(), nil
	}

	return "", fmt.Errorf("zalo personal protocol: parse send result: unsupported msgId format")
}

func getServiceURL(sess *Session, service string) string {
	if sess == nil || sess.LoginInfo == nil {
		return ""
	}

	var urls []string
	switch service {
	case "chat":
		urls = sess.LoginInfo.ZpwServiceMapV3.Chat
	case "group":
		urls = sess.LoginInfo.ZpwServiceMapV3.Group
	case "file":
		urls = sess.LoginInfo.ZpwServiceMapV3.File
	case "profile":
		urls = sess.LoginInfo.ZpwServiceMapV3.Profile
	case "group_poll":
		urls = sess.LoginInfo.ZpwServiceMapV3.GroupPoll
	}
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}

func encryptPayload(sess *Session, payload map[string]any) (string, error) {
	if sess == nil {
		return "", fmt.Errorf("zalo personal protocol: session is required")
	}
	key := SecretKey(sess.SecretKey).Bytes()
	if key == nil {
		return "", fmt.Errorf("zalo personal protocol: invalid session secret key")
	}

	blob, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return EncodeAESCBC(key, string(blob), false)
}

func decryptDataField(sess *Session, data string) ([]byte, error) {
	if sess == nil {
		return nil, fmt.Errorf("zalo personal protocol: session is required")
	}
	key := SecretKey(sess.SecretKey).Bytes()
	if key == nil {
		return nil, fmt.Errorf("zalo personal protocol: invalid session secret key")
	}
	unescaped, err := url.PathUnescape(data)
	if err != nil {
		return nil, err
	}
	plain, err := DecodeAESCBC(key, unescaped)
	if err != nil {
		return nil, err
	}

	var inner Response[json.RawMessage]
	if err := json.Unmarshal(plain, &inner); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: unwrap inner response: %w", err)
	}
	if inner.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo personal protocol: inner error code %d: %s", inner.ErrorCode, inner.ErrorMessage)
	}
	return inner.Data, nil
}
