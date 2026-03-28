package protocol

import (
	"context"
	"fmt"
	"strings"
)

const destTypeUser = 3

func SendTyping(ctx context.Context, sess *Session, threadID string, threadType ThreadType) error {
	if sess == nil {
		return fmt.Errorf("zalo personal protocol: session is required")
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return fmt.Errorf("zalo personal protocol: thread ID is required")
	}

	serviceKey := "chat"
	apiPath := "/api/message/typing"
	payload := map[string]any{
		"imei": sess.IMEI,
	}
	if threadType == ThreadTypeGroup {
		serviceKey = "group"
		apiPath = "/api/group/typing"
		payload["grid"] = threadID
	} else {
		payload["toid"] = threadID
		payload["destType"] = destTypeUser
	}

	baseURL := getServiceURL(sess, serviceKey)
	if baseURL == "" {
		return fmt.Errorf("zalo personal protocol: no service URL for %s", serviceKey)
	}
	return postEncryptedAction(ctx, sess, baseURL+apiPath, payload, "typing")
}
