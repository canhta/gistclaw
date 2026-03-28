package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type UnreadMarkInfo struct {
	ThreadID   string
	ThreadType ThreadType
	MarkedAt   time.Time
}

type PinnedConversationInfo struct {
	ThreadID   string
	ThreadType ThreadType
}

type HiddenConversationInfo struct {
	ThreadID   string
	ThreadType ThreadType
}

type ArchivedConversationInfo struct {
	ThreadID   string
	ThreadType ThreadType
}

func FetchUnreadMarks(ctx context.Context, sess *Session) ([]UnreadMarkInfo, error) {
	baseURL := getServiceURL(sess, "conversation")
	if baseURL == "" {
		return nil, fmt.Errorf("zalo_personal: no conversation service URL")
	}

	reqURL := makeURL(sess, baseURL+"/api/conv/getUnreadMark", nil, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: fetch unread marks: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse unread marks response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo_personal: unread marks error code %d: %s", envelope.ErrorCode, envelope.ErrorMessage)
	}
	if envelope.Data == nil {
		return nil, nil
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: decrypt unread marks: %w", err)
	}

	var payload struct {
		ConvsGroup []struct {
			ID int64 `json:"id"`
			TS int64 `json:"ts"`
		} `json:"convsGroup"`
		ConvsUser []struct {
			ID int64 `json:"id"`
			TS int64 `json:"ts"`
		} `json:"convsUser"`
	}
	if err := unmarshalProtocolData(plain, &payload); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse unread marks payload: %w", err)
	}

	marks := make([]UnreadMarkInfo, 0, len(payload.ConvsGroup)+len(payload.ConvsUser))
	for _, mark := range payload.ConvsUser {
		threadID := strings.TrimSpace(fmt.Sprintf("%d", mark.ID))
		if threadID == "" || threadID == "0" {
			continue
		}
		marks = append(marks, UnreadMarkInfo{
			ThreadID:   threadID,
			ThreadType: ThreadTypeUser,
			MarkedAt:   time.UnixMilli(mark.TS).UTC(),
		})
	}
	for _, mark := range payload.ConvsGroup {
		threadID := strings.TrimSpace(fmt.Sprintf("%d", mark.ID))
		if threadID == "" || threadID == "0" {
			continue
		}
		marks = append(marks, UnreadMarkInfo{
			ThreadID:   threadID,
			ThreadType: ThreadTypeGroup,
			MarkedAt:   time.UnixMilli(mark.TS).UTC(),
		})
	}
	return marks, nil
}

func FetchPinnedConversations(ctx context.Context, sess *Session) ([]PinnedConversationInfo, error) {
	baseURL := getServiceURL(sess, "conversation")
	if baseURL == "" {
		return nil, fmt.Errorf("zalo_personal: no conversation service URL")
	}

	params := map[string]any{"imei": sess.IMEI}
	encData, err := encryptPayload(sess, params)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: encrypt pinned conversations payload: %w", err)
	}

	reqURL := makeURL(sess, baseURL+"/api/pinconvers/list", map[string]any{"params": encData}, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: fetch pinned conversations: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse pinned conversations response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo_personal: pinned conversations error code %d: %s", envelope.ErrorCode, envelope.ErrorMessage)
	}
	if envelope.Data == nil {
		return nil, nil
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: decrypt pinned conversations: %w", err)
	}

	var payload struct {
		Conversations []string `json:"conversations"`
	}
	if err := unmarshalProtocolData(plain, &payload); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse pinned conversations payload: %w", err)
	}

	items := make([]PinnedConversationInfo, 0, len(payload.Conversations))
	for _, raw := range payload.Conversations {
		threadID, threadType, ok := decodeConversationThread(raw)
		if !ok {
			continue
		}
		items = append(items, PinnedConversationInfo{ThreadID: threadID, ThreadType: threadType})
	}
	return items, nil
}

func FetchHiddenConversations(ctx context.Context, sess *Session) ([]HiddenConversationInfo, error) {
	baseURL := getServiceURL(sess, "conversation")
	if baseURL == "" {
		return nil, fmt.Errorf("zalo_personal: no conversation service URL")
	}

	params := map[string]any{"imei": sess.IMEI}
	encData, err := encryptPayload(sess, params)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: encrypt hidden conversations payload: %w", err)
	}

	reqURL := makeURL(sess, baseURL+"/api/hiddenconvers/get-all", map[string]any{"params": encData}, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: fetch hidden conversations: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse hidden conversations response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo_personal: hidden conversations error code %d: %s", envelope.ErrorCode, envelope.ErrorMessage)
	}
	if envelope.Data == nil {
		return nil, nil
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: decrypt hidden conversations: %w", err)
	}

	var payload struct {
		Threads []struct {
			IsGroup  int    `json:"is_group"`
			ThreadID string `json:"thread_id"`
		} `json:"threads"`
	}
	if err := unmarshalProtocolData(plain, &payload); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse hidden conversations payload: %w", err)
	}

	items := make([]HiddenConversationInfo, 0, len(payload.Threads))
	for _, thread := range payload.Threads {
		threadID := strings.TrimSpace(thread.ThreadID)
		if threadID == "" {
			continue
		}
		threadType := ThreadTypeUser
		if thread.IsGroup == 1 {
			threadType = ThreadTypeGroup
		}
		items = append(items, HiddenConversationInfo{ThreadID: threadID, ThreadType: threadType})
	}
	return items, nil
}

func FetchArchivedConversations(ctx context.Context, sess *Session) ([]ArchivedConversationInfo, error) {
	baseURL := getServiceURL(sess, "label")
	if baseURL == "" {
		return nil, fmt.Errorf("zalo_personal: no label service URL")
	}

	params := map[string]any{
		"version": 1,
		"imei":    sess.IMEI,
	}
	encData, err := encryptPayload(sess, params)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: encrypt archived conversations payload: %w", err)
	}

	reqURL := makeURL(sess, baseURL+"/api/archivedchat/list", map[string]any{"params": encData}, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: fetch archived conversations: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse archived conversations response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo_personal: archived conversations error code %d: %s", envelope.ErrorCode, envelope.ErrorMessage)
	}
	if envelope.Data == nil {
		return nil, nil
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: decrypt archived conversations: %w", err)
	}

	var payload struct {
		Items []any `json:"items"`
	}
	if err := unmarshalProtocolData(plain, &payload); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse archived conversations payload: %w", err)
	}

	items := make([]ArchivedConversationInfo, 0, len(payload.Items))
	for _, raw := range payload.Items {
		threadID, threadType, ok := decodeArchivedConversation(raw)
		if !ok {
			continue
		}
		items = append(items, ArchivedConversationInfo{ThreadID: threadID, ThreadType: threadType})
	}
	return items, nil
}

func unmarshalProtocolData[T any](plain []byte, target *T) error {
	payload := bytesTrimSpace(plain)
	if len(payload) == 0 {
		return nil
	}
	if payload[0] == '"' {
		var inner string
		if err := json.Unmarshal(payload, &inner); err != nil {
			return err
		}
		payload = []byte(inner)
	}
	return json.Unmarshal(payload, target)
}

func bytesTrimSpace(data []byte) []byte {
	return []byte(strings.TrimSpace(string(data)))
}

func decodeConversationThread(raw string) (string, ThreadType, bool) {
	value := strings.TrimSpace(raw)
	if len(value) < 2 {
		return "", 0, false
	}
	switch strings.ToLower(value[:1]) {
	case "u":
		return strings.TrimSpace(value[1:]), ThreadTypeUser, true
	case "g":
		return strings.TrimSpace(value[1:]), ThreadTypeGroup, true
	default:
		return "", 0, false
	}
}

func decodeArchivedConversation(raw any) (string, ThreadType, bool) {
	switch value := raw.(type) {
	case string:
		return decodeConversationThread(value)
	case map[string]any:
		if threadID, threadType, ok := decodeArchivedConversationMap(value); ok {
			return threadID, threadType, true
		}
	default:
	}
	return "", 0, false
}

func decodeArchivedConversationMap(value map[string]any) (string, ThreadType, bool) {
	for _, key := range []string{"id", "thread_id", "threadId", "conversationId"} {
		raw, ok := value[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case string:
			if threadID, threadType, ok := decodeConversationThread(v); ok {
				return threadID, threadType, true
			}
			threadID := strings.TrimSpace(v)
			if threadID == "" {
				continue
			}
			if isGroup, ok := mapIsGroup(value); ok && isGroup {
				return threadID, ThreadTypeGroup, true
			}
			return threadID, ThreadTypeUser, true
		case float64:
			threadID := strings.TrimSpace(fmt.Sprintf("%.0f", v))
			if threadID == "" {
				continue
			}
			if isGroup, ok := mapIsGroup(value); ok && isGroup {
				return threadID, ThreadTypeGroup, true
			}
			return threadID, ThreadTypeUser, true
		}
	}
	return "", 0, false
}

func mapIsGroup(value map[string]any) (bool, bool) {
	for _, key := range []string{"is_group", "isGroup"} {
		raw, ok := value[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case bool:
			return v, true
		case float64:
			return v != 0, true
		case string:
			switch strings.TrimSpace(strings.ToLower(v)) {
			case "1", "true", "group":
				return true, true
			case "0", "false", "user", "contact":
				return false, true
			}
		}
	}
	return false, false
}
