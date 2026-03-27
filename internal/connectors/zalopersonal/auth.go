package zalopersonal

import (
	"fmt"
	"strings"
)

type StoredCredentials struct {
	AccountID   string `json:"account_id"`
	DisplayName string `json:"display_name"`
	IMEI        string `json:"imei"`
	Cookie      string `json:"cookie"`
	UserAgent   string `json:"user_agent"`
	Language    string `json:"language"`
}

func (c StoredCredentials) Validate() error {
	switch {
	case strings.TrimSpace(c.AccountID) == "":
		return fmt.Errorf("zalo personal: account_id is required")
	case strings.TrimSpace(c.IMEI) == "":
		return fmt.Errorf("zalo personal: imei is required")
	case strings.TrimSpace(c.Cookie) == "":
		return fmt.Errorf("zalo personal: cookie is required")
	case strings.TrimSpace(c.UserAgent) == "":
		return fmt.Errorf("zalo personal: user_agent is required")
	default:
		return nil
	}
}

func (c StoredCredentials) normalized() StoredCredentials {
	c.AccountID = strings.TrimSpace(c.AccountID)
	c.DisplayName = strings.TrimSpace(c.DisplayName)
	c.IMEI = strings.TrimSpace(c.IMEI)
	c.Cookie = strings.TrimSpace(c.Cookie)
	c.UserAgent = strings.TrimSpace(c.UserAgent)
	c.Language = strings.TrimSpace(c.Language)
	return c
}
