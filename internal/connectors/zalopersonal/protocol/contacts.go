package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type FriendInfo struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	ZaloName    string `json:"zaloName,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}

func FetchFriends(ctx context.Context, sess *Session) ([]FriendInfo, error) {
	baseURL := getServiceURL(sess, "profile")
	if baseURL == "" {
		return nil, fmt.Errorf("zalo personal protocol: no profile service URL")
	}

	payload := map[string]any{
		"page":        1,
		"count":       20000,
		"incInvalid":  1,
		"avatar_size": 120,
		"actiontime":  0,
		"imei":        sess.IMEI,
	}

	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: encrypt friends payload: %w", err)
	}

	reqURL := makeURL(sess, baseURL+"/api/social/friend/getfriends", map[string]any{
		"params": encData,
	}, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: fetch friends: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: parse friends response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo personal protocol: friends error code %d: %s", envelope.ErrorCode, envelope.ErrorMessage)
	}
	if envelope.Data == nil {
		return nil, fmt.Errorf("zalo personal protocol: empty friends data")
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: decrypt friends: %w", err)
	}

	var friends []FriendInfo
	if err := json.Unmarshal(plain, &friends); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: parse friends list: %w", err)
	}
	return friends, nil
}
