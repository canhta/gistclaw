package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
)

type FriendInfo struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	ZaloName    string `json:"zaloName,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}

type GroupListInfo struct {
	GroupID     string `json:"groupId"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar,omitempty"`
	TotalMember int    `json:"totalMember"`
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

func FetchGroups(ctx context.Context, sess *Session) ([]GroupListInfo, error) {
	gridVerMap, err := fetchGroupIDs(ctx, sess)
	if err != nil {
		return nil, err
	}
	if len(gridVerMap) == 0 {
		return nil, nil
	}
	return fetchGroupDetails(ctx, sess, gridVerMap)
}

func fetchGroupIDs(ctx context.Context, sess *Session) (map[string]string, error) {
	baseURL := getServiceURL(sess, "group_poll")
	if baseURL == "" {
		return nil, fmt.Errorf("zalo personal protocol: no group_poll service URL")
	}

	reqURL := makeURL(sess, baseURL+"/api/group/getlg/v4", nil, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: fetch group IDs: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: parse group IDs response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo personal protocol: group IDs error code %d: %s", envelope.ErrorCode, envelope.ErrorMessage)
	}
	if envelope.Data == nil {
		return nil, nil
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: decrypt group IDs: %w", err)
	}
	var result struct {
		GridVerMap map[string]string `json:"gridVerMap"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: parse group IDs: %w", err)
	}
	return result.GridVerMap, nil
}

func fetchGroupDetails(ctx context.Context, sess *Session, gridVerMap map[string]string) ([]GroupListInfo, error) {
	baseURL := getServiceURL(sess, "group")
	if baseURL == "" {
		return nil, fmt.Errorf("zalo personal protocol: no group service URL")
	}

	zeroVerMap := make(map[string]int, len(gridVerMap))
	for id := range gridVerMap {
		zeroVerMap[id] = 0
	}
	gridVerJSON, err := json.Marshal(zeroVerMap)
	if err != nil {
		return nil, err
	}

	encData, err := encryptPayload(sess, map[string]any{"gridVerMap": string(gridVerJSON)})
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: encrypt group details payload: %w", err)
	}

	reqURL := makeURL(sess, baseURL+"/api/group/getmg-v2", nil, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, buildFormBody(map[string]string{"params": encData}))
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: fetch group details: %w", err)
	}
	defer resp.Body.Close()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: parse group details response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo personal protocol: group details error code %d: %s", envelope.ErrorCode, envelope.ErrorMessage)
	}
	if envelope.Data == nil {
		return nil, nil
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo personal protocol: decrypt group details: %w", err)
	}
	var result struct {
		GridInfoMap map[string]struct {
			Name        string `json:"name"`
			Avatar      string `json:"avt"`
			TotalMember int    `json:"totalMember"`
		} `json:"gridInfoMap"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		return nil, fmt.Errorf("zalo personal protocol: parse group details: %w", err)
	}

	groups := make([]GroupListInfo, 0, len(result.GridInfoMap))
	for id, info := range result.GridInfoMap {
		groups = append(groups, GroupListInfo{
			GroupID:     id,
			Name:        info.Name,
			Avatar:      info.Avatar,
			TotalMember: info.TotalMember,
		})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups, nil
}
