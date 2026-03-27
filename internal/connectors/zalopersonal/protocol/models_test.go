package protocol

import (
	"encoding/json"
	"testing"
)

func TestServerInfoUnmarshalJSONSupportsSettingsTypo(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"data": {
			"setttings": {
				"features": {
					"socket": {
						"ping_interval": 15
					}
				},
				"keepalive": {
					"alway_keepalive": 1,
					"keepalive_duration": 30
				}
			}
		}
	}`)

	var result struct {
		Data ServerInfo `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal server info: %v", err)
	}
	if result.Data.Settings == nil {
		t.Fatal("expected settings to be populated")
	}
	if result.Data.Settings.Features.Socket.PingInterval != 15 {
		t.Fatalf("expected ping interval 15, got %d", result.Data.Settings.Features.Socket.PingInterval)
	}
}

func TestSocketRetryConfigUnmarshalJSONSupportsSingleIntTimes(t *testing.T) {
	t.Parallel()

	var cfg SocketRetryConfig
	if err := json.Unmarshal([]byte(`{"max":3,"times":5}`), &cfg); err != nil {
		t.Fatalf("unmarshal retry config: %v", err)
	}
	if cfg.Max == nil || *cfg.Max != 3 {
		t.Fatalf("expected max=3, got %+v", cfg.Max)
	}
	if len(cfg.Times) != 1 || cfg.Times[0] != 5 {
		t.Fatalf("expected times [5], got %+v", cfg.Times)
	}
}
