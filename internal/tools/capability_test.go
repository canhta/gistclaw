package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime/capabilities"
)

func TestRegisterCapabilityTools_RegistersCapabilityTools(t *testing.T) {
	reg := NewRegistry()
	RegisterCapabilityTools(reg, CapabilityHandlers{
		DirectoryList: func(context.Context, capabilities.DirectoryListRequest) (capabilities.DirectoryListResult, error) {
			return capabilities.DirectoryListResult{}, nil
		},
		ResolveTarget: func(context.Context, capabilities.TargetResolveRequest) (capabilities.TargetResolveResult, error) {
			return capabilities.TargetResolveResult{}, nil
		},
		Send: func(context.Context, capabilities.SendRequest) (capabilities.SendResult, error) {
			return capabilities.SendResult{}, nil
		},
		Status: func(context.Context, capabilities.StatusRequest) (capabilities.StatusResult, error) {
			return capabilities.StatusResult{}, nil
		},
		AppAction: func(context.Context, capabilities.AppActionRequest) (capabilities.AppActionResult, error) {
			return capabilities.AppActionResult{}, nil
		},
	})

	for _, name := range []string{
		"connector_directory_list",
		"connector_target_resolve",
		"connector_send",
		"connector_status",
		"app_action",
	} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("expected %q to be registered", name)
		}
	}
}

func TestConnectorDirectoryListTool_InvokeNormalizesOutput(t *testing.T) {
	tool := &ConnectorDirectoryListTool{
		list: func(_ context.Context, req capabilities.DirectoryListRequest) (capabilities.DirectoryListResult, error) {
			if req.ConnectorID != "zalo_personal" || req.Scope != "contacts" || req.Query != "anh" || req.Limit != 5 {
				t.Fatalf("unexpected request: %+v", req)
			}
			return capabilities.DirectoryListResult{
				ConnectorID: "zalo_personal",
				Scope:       "contacts",
				Entries: []capabilities.DirectoryEntry{
					{ID: "user-1", Title: "Anh An", Kind: "contact"},
				},
			}, nil
		},
	}

	result, err := tool.Invoke(context.Background(), model.ToolCall{
		ID:        "call-1",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"connector_id":"zalo_personal","scope":"contacts","query":"anh","limit":5}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var payload capabilities.DirectoryListResult
	if err := json.Unmarshal([]byte(result.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.ConnectorID != "zalo_personal" || len(payload.Entries) != 1 {
		t.Fatalf("unexpected output payload: %+v", payload)
	}
}

func TestConnectorSendTool_InvokeNormalizesOutput(t *testing.T) {
	tool := &ConnectorSendTool{
		send: func(_ context.Context, req capabilities.SendRequest) (capabilities.SendResult, error) {
			if req.ConnectorID != "zalo_personal" || req.TargetID != "user-1" || req.TargetType != "contact" || req.Message != "hello" {
				t.Fatalf("unexpected request: %+v", req)
			}
			return capabilities.SendResult{
				ConnectorID: "zalo_personal",
				TargetID:    "user-1",
				TargetType:  "contact",
				Accepted:    true,
				Summary:     "message accepted",
			}, nil
		},
	}

	result, err := tool.Invoke(context.Background(), model.ToolCall{
		ID:        "call-1",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"connector_id":"zalo_personal","target_id":"user-1","target_type":"contact","message":"hello"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var payload capabilities.SendResult
	if err := json.Unmarshal([]byte(result.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !payload.Accepted || payload.TargetID != "user-1" {
		t.Fatalf("unexpected output payload: %+v", payload)
	}
}

func TestAppActionTool_InvokeNormalizesOutput(t *testing.T) {
	tool := &AppActionTool{
		action: func(_ context.Context, req capabilities.AppActionRequest) (capabilities.AppActionResult, error) {
			if req.Name != "status" {
				t.Fatalf("unexpected request: %+v", req)
			}
			return capabilities.AppActionResult{
				Name:    "status",
				Summary: "status loaded",
				Data: map[string]any{
					"active_runs": float64(2),
				},
			}, nil
		},
	}

	result, err := tool.Invoke(context.Background(), model.ToolCall{
		ID:        "call-1",
		ToolName:  tool.Name(),
		InputJSON: []byte(`{"name":"status"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var payload capabilities.AppActionResult
	if err := json.Unmarshal([]byte(result.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Name != "status" || payload.Summary != "status loaded" {
		t.Fatalf("unexpected output payload: %+v", payload)
	}
}
