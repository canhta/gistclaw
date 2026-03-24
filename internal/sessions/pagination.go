package sessions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type PageResult[T any] struct {
	Items      []T
	NextCursor string
	PrevCursor string
	HasNext    bool
	HasPrev    bool
}

type sessionPageCursor struct {
	UpdatedAtMicros int64  `json:"updated_at_micros"`
	RoleRank        int    `json:"role_rank"`
	CreatedAtMicros int64  `json:"created_at_micros"`
	ID              string `json:"id"`
}

type routePageCursor struct {
	CreatedAtMicros int64  `json:"created_at_micros"`
	ID              string `json:"id"`
}

type deliveryPageCursor struct {
	StatusRank      int    `json:"status_rank"`
	CreatedAtMicros int64  `json:"created_at_micros"`
	ID              string `json:"id"`
}

func normalizePageDirection(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "prev":
		return "prev"
	default:
		return "next"
	}
}

func sqliteMicros(expr string) string {
	normalized := fmt.Sprintf(`CASE
		WHEN %s LIKE '%% UTC' THEN substr(%s, 1, length(%s) - 10)
		ELSE %s
	END`, expr, expr, expr, expr)
	return fmt.Sprintf("CAST(ROUND((julianday(%s) - 2440587.5) * 86400000000.0) AS INTEGER)", normalized)
}

func encodePageCursor(payload any) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodePageCursor(raw string, dest any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("decode cursor: %w", err)
	}
	if err := json.Unmarshal(decoded, dest); err != nil {
		return fmt.Errorf("unmarshal cursor: %w", err)
	}
	return nil
}

func reverseSlice[T any](items []T) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
