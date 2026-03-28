package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func UpdateUnreadMark(ctx context.Context, sess *Session, threadID string, threadType ThreadType, unread bool) error {
	baseURL := getServiceURL(sess, "conversation")
	if baseURL == "" {
		return fmt.Errorf("zalo personal protocol: no conversation service URL")
	}

	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return fmt.Errorf("zalo personal protocol: thread ID is required")
	}

	timestamp := time.Now().UnixMilli()
	params := map[string]any{}
	if unread {
		item := map[string]any{
			"id":       threadID,
			"cliMsgId": fmt.Sprintf("%d", timestamp),
			"fromUid":  "0",
			"ts":       timestamp,
		}
		if threadType == ThreadTypeGroup {
			params["convsGroup"] = []any{item}
			params["convsUser"] = []any{}
		} else {
			params["convsUser"] = []any{item}
			params["convsGroup"] = []any{}
		}
		params["imei"] = sess.IMEI
		paramJSON, err := marshalJSONText(params)
		if err != nil {
			return err
		}
		return postConversationState(ctx, sess, baseURL+"/api/conv/addUnreadMark", map[string]any{
			"param": paramJSON,
		})
	}

	dataItem := map[string]any{
		"id": threadID,
		"ts": timestamp,
	}
	if threadType == ThreadTypeGroup {
		params["convsGroup"] = []any{threadID}
		params["convsUser"] = []any{}
		params["convsGroupData"] = []any{dataItem}
		params["convsUserData"] = []any{}
	} else {
		params["convsUser"] = []any{threadID}
		params["convsGroup"] = []any{}
		params["convsUserData"] = []any{dataItem}
		params["convsGroupData"] = []any{}
	}
	paramJSON, err := marshalJSONText(params)
	if err != nil {
		return err
	}
	return postConversationState(ctx, sess, baseURL+"/api/conv/removeUnreadMark", map[string]any{"param": paramJSON})
}

func UpdatePinnedConversation(ctx context.Context, sess *Session, threadID string, threadType ThreadType, enabled bool) error {
	baseURL := getServiceURL(sess, "conversation")
	if baseURL == "" {
		return fmt.Errorf("zalo personal protocol: no conversation service URL")
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return fmt.Errorf("zalo personal protocol: thread ID is required")
	}

	prefix := "u"
	if threadType == ThreadTypeGroup {
		prefix = "g"
	}
	return postConversationState(ctx, sess, baseURL+"/api/pinconvers/updatev2", map[string]any{
		"actionType": map[bool]int{true: 1, false: 2}[enabled],
		"conversations": []string{
			prefix + threadID,
		},
	})
}

func UpdateArchivedConversation(ctx context.Context, sess *Session, threadID string, threadType ThreadType, enabled bool) error {
	baseURL := getServiceURL(sess, "label")
	if baseURL == "" {
		return fmt.Errorf("zalo personal protocol: no label service URL")
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return fmt.Errorf("zalo personal protocol: thread ID is required")
	}

	typeValue := 0
	if threadType == ThreadTypeGroup {
		typeValue = 1
	}
	return postConversationState(ctx, sess, baseURL+"/api/archivedchat/update", map[string]any{
		"actionType": map[bool]int{true: 0, false: 1}[enabled],
		"ids": []map[string]any{
			{
				"id":   threadID,
				"type": typeValue,
			},
		},
		"imei":    sess.IMEI,
		"version": time.Now().UnixMilli(),
	})
}

func UpdateHiddenConversation(ctx context.Context, sess *Session, threadID string, threadType ThreadType, enabled bool) error {
	baseURL := getServiceURL(sess, "conversation")
	if baseURL == "" {
		return fmt.Errorf("zalo personal protocol: no conversation service URL")
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return fmt.Errorf("zalo personal protocol: thread ID is required")
	}

	threads := []map[string]any{{
		"thread_id": threadID,
		"is_group":  map[bool]int{true: 1, false: 0}[threadType == ThreadTypeGroup],
	}}
	threadsJSON, err := marshalJSONText(threads)
	if err != nil {
		return err
	}
	params := map[string]any{
		"imei": sess.IMEI,
	}
	if enabled {
		params["add_threads"] = threadsJSON
		params["del_threads"] = "[]"
	} else {
		params["add_threads"] = "[]"
		params["del_threads"] = threadsJSON
	}
	return postConversationState(ctx, sess, baseURL+"/api/hiddenconvers/add-remove", params)
}

func postConversationState(ctx context.Context, sess *Session, reqURL string, payload map[string]any) error {
	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, makeURL(sess, reqURL, nil, true), buildFormBody(map[string]string{
		"params": encData,
	}))
	if err != nil {
		return err
	}
	setDefaultHeaders(req, sess)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := sess.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var envelope Response[json.RawMessage]
	if err := readJSON(resp, &envelope); err != nil {
		return err
	}
	if envelope.ErrorCode != 0 {
		return fmt.Errorf("zalo personal protocol: conversation state error code %d: %s", envelope.ErrorCode, envelope.ErrorMessage)
	}
	return nil
}

func marshalJSONText(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
