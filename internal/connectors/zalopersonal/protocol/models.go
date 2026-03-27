package protocol

import (
	"encoding/base64"
	"encoding/json"
)

type SecretKey string

func (s SecretKey) Bytes() []byte {
	decoded, err := base64.StdEncoding.DecodeString(string(s))
	if err != nil {
		return nil
	}
	return decoded
}

type LoginInfo struct {
	UID             string          `json:"uid"`
	ZPWEnk          string          `json:"zpw_enk"`
	ZpwWebsocket    []string        `json:"zpw_ws"`
	ZpwServiceMapV3 ZpwServiceMapV3 `json:"zpw_service_map_v3"`
}

type ZpwServiceMapV3 struct {
	Chat      []string `json:"chat"`
	Group     []string `json:"group"`
	File      []string `json:"file"`
	Profile   []string `json:"profile"`
	GroupPoll []string `json:"group_poll"`
}

type ServerInfo struct {
	Settings *Settings `json:"settings"`
}

func (s *ServerInfo) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, key := range []string{"settings", "setttings"} {
		if payload, ok := raw[key]; ok {
			return json.Unmarshal(payload, &s.Settings)
		}
	}
	return nil
}

type Settings struct {
	Features  Features          `json:"features"`
	Keepalive KeepaliveSettings `json:"keepalive"`
}

type Features struct {
	Socket SocketSettings `json:"socket"`
}

type SocketSettings struct {
	PingInterval     int                          `json:"ping_interval"`
	Retries          map[string]SocketRetryConfig `json:"retries"`
	CloseAndRetry    []int                        `json:"close_and_retry_codes"`
	RotateErrorCodes []int                        `json:"rotate_error_codes"`
}

type SocketRetryConfig struct {
	Max   *int  `json:"max,omitempty"`
	Times []int `json:"times"`
}

func (r *SocketRetryConfig) UnmarshalJSON(data []byte) error {
	type rawRetry struct {
		Max   *int            `json:"max,omitempty"`
		Times json.RawMessage `json:"times"`
	}
	var raw rawRetry
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Max = raw.Max
	if err := json.Unmarshal(raw.Times, &r.Times); err == nil {
		return nil
	}
	var single int
	if err := json.Unmarshal(raw.Times, &single); err != nil {
		return err
	}
	r.Times = []int{single}
	return nil
}

type KeepaliveSettings struct {
	AlwaysKeepalive   uint `json:"alway_keepalive"`
	KeepaliveDuration uint `json:"keepalive_duration"`
}

type Response[T any] struct {
	ErrorCode    int    `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Data         T      `json:"data"`
}

type QRGeneratedData struct {
	Code  string `json:"code"`
	Image string `json:"image"`
}

type QRScannedData struct {
	Avatar      string `json:"avatar"`
	DisplayName string `json:"display_name"`
}

type QRUserInfo struct {
	Logged bool     `json:"logged"`
	Info   UserInfo `json:"info"`
}

type UserInfo struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}
