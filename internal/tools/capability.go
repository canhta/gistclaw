package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/canhta/gistclaw/internal/model"
	"github.com/canhta/gistclaw/internal/runtime/capabilities"
)

type CapabilityHandlers struct {
	DirectoryList func(context.Context, capabilities.DirectoryListRequest) (capabilities.DirectoryListResult, error)
	ResolveTarget func(context.Context, capabilities.TargetResolveRequest) (capabilities.TargetResolveResult, error)
	Send          func(context.Context, capabilities.SendRequest) (capabilities.SendResult, error)
	Status        func(context.Context, capabilities.StatusRequest) (capabilities.StatusResult, error)
	AppAction     func(context.Context, capabilities.AppActionRequest) (capabilities.AppActionResult, error)
}

type ConnectorDirectoryListTool struct {
	list func(context.Context, capabilities.DirectoryListRequest) (capabilities.DirectoryListResult, error)
}

type ConnectorTargetResolveTool struct {
	resolve func(context.Context, capabilities.TargetResolveRequest) (capabilities.TargetResolveResult, error)
}

type ConnectorSendTool struct {
	send func(context.Context, capabilities.SendRequest) (capabilities.SendResult, error)
}

type ConnectorStatusTool struct {
	status func(context.Context, capabilities.StatusRequest) (capabilities.StatusResult, error)
}

type AppActionTool struct {
	action func(context.Context, capabilities.AppActionRequest) (capabilities.AppActionResult, error)
}

func RegisterCapabilityTools(reg *Registry, handlers CapabilityHandlers) {
	if reg == nil {
		return
	}
	if handlers.DirectoryList != nil {
		reg.Register(&ConnectorDirectoryListTool{list: handlers.DirectoryList})
	}
	if handlers.ResolveTarget != nil {
		reg.Register(&ConnectorTargetResolveTool{resolve: handlers.ResolveTarget})
	}
	if handlers.Send != nil {
		reg.Register(&ConnectorSendTool{send: handlers.Send})
	}
	if handlers.Status != nil {
		reg.Register(&ConnectorStatusTool{status: handlers.Status})
	}
	if handlers.AppAction != nil {
		reg.Register(&AppActionTool{action: handlers.AppAction})
	}
}

func (t *ConnectorDirectoryListTool) Name() string { return "connector_directory_list" }

func (t *ConnectorDirectoryListTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "List direct connector directory entries such as contacts or groups.",
		InputSchemaJSON: `{"type":"object","properties":{"connector_id":{"type":"string"},"scope":{"type":"string"},"query":{"type":"string"},"limit":{"type":"integer","minimum":1}},"required":["connector_id"]}`,
		Family:          model.ToolFamilyConnectorCapability,
		Intents:         []model.ToolIntent{model.ToolIntentDirectoryList},
		Risk:            model.RiskLow,
		SideEffect:      effectRead,
	}
}

func (t *ConnectorDirectoryListTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	if t.list == nil {
		return model.ToolResult{}, fmt.Errorf("connector_directory_list: handler is required")
	}
	var input capabilities.DirectoryListRequest
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("connector_directory_list: decode input: %w", err)
	}
	if input.ConnectorID == "" {
		return model.ToolResult{}, fmt.Errorf("connector_directory_list: connector_id is required")
	}
	result, err := t.list(ctx, input)
	if err != nil {
		return model.ToolResult{}, err
	}
	return marshalCapabilityResult("connector_directory_list", result)
}

func (t *ConnectorTargetResolveTool) Name() string { return "connector_target_resolve" }

func (t *ConnectorTargetResolveTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Resolve a user-provided connector target name or query into concrete targets.",
		InputSchemaJSON: `{"type":"object","properties":{"connector_id":{"type":"string"},"query":{"type":"string"},"scope":{"type":"string"},"limit":{"type":"integer","minimum":1}},"required":["connector_id","query"]}`,
		Family:          model.ToolFamilyConnectorCapability,
		Intents:         []model.ToolIntent{model.ToolIntentTargetResolve},
		Risk:            model.RiskLow,
		SideEffect:      effectRead,
	}
}

func (t *ConnectorTargetResolveTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	if t.resolve == nil {
		return model.ToolResult{}, fmt.Errorf("connector_target_resolve: handler is required")
	}
	var input capabilities.TargetResolveRequest
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("connector_target_resolve: decode input: %w", err)
	}
	if input.ConnectorID == "" {
		return model.ToolResult{}, fmt.Errorf("connector_target_resolve: connector_id is required")
	}
	if input.Query == "" {
		return model.ToolResult{}, fmt.Errorf("connector_target_resolve: query is required")
	}
	result, err := t.resolve(ctx, input)
	if err != nil {
		return model.ToolResult{}, err
	}
	return marshalCapabilityResult("connector_target_resolve", result)
}

func (t *ConnectorSendTool) Name() string { return "connector_send" }

func (t *ConnectorSendTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Send a direct message through a registered connector target.",
		InputSchemaJSON: `{"type":"object","properties":{"connector_id":{"type":"string"},"target_id":{"type":"string"},"target_type":{"type":"string"},"message":{"type":"string"},"metadata":{"type":"object","additionalProperties":{"type":"string"}}},"required":["connector_id","target_id","message"]}`,
		Family:          model.ToolFamilyConnectorCapability,
		Intents:         []model.ToolIntent{model.ToolIntentMessageSend},
		Risk:            model.RiskMedium,
		SideEffect:      "connector_send",
		Approval:        "required",
	}
}

func (t *ConnectorSendTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	if t.send == nil {
		return model.ToolResult{}, fmt.Errorf("connector_send: handler is required")
	}
	var input capabilities.SendRequest
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("connector_send: decode input: %w", err)
	}
	if input.ConnectorID == "" {
		return model.ToolResult{}, fmt.Errorf("connector_send: connector_id is required")
	}
	if input.TargetID == "" {
		return model.ToolResult{}, fmt.Errorf("connector_send: target_id is required")
	}
	if input.Message == "" {
		return model.ToolResult{}, fmt.Errorf("connector_send: message is required")
	}
	result, err := t.send(ctx, input)
	if err != nil {
		return model.ToolResult{}, err
	}
	return marshalCapabilityResult("connector_send", result)
}

func (t *ConnectorStatusTool) Name() string { return "connector_status" }

func (t *ConnectorStatusTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Read connector health and readiness state.",
		InputSchemaJSON: `{"type":"object","properties":{"connector_id":{"type":"string"}}}`,
		Family:          model.ToolFamilyConnectorCapability,
		Intents:         []model.ToolIntent{model.ToolIntentStatusRead},
		Risk:            model.RiskLow,
		SideEffect:      effectRead,
	}
}

func (t *ConnectorStatusTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	if t.status == nil {
		return model.ToolResult{}, fmt.Errorf("connector_status: handler is required")
	}
	var input capabilities.StatusRequest
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("connector_status: decode input: %w", err)
	}
	result, err := t.status(ctx, input)
	if err != nil {
		return model.ToolResult{}, err
	}
	return marshalCapabilityResult("connector_status", result)
}

func (t *AppActionTool) Name() string { return "app_action" }

func (t *AppActionTool) Spec() model.ToolSpec {
	return model.ToolSpec{
		Name:            t.Name(),
		Description:     "Run a deterministic built-in app action such as system status.",
		InputSchemaJSON: `{"type":"object","properties":{"name":{"type":"string"},"arguments":{"type":"object"}},"required":["name"]}`,
		Family:          model.ToolFamilyRuntimeCapability,
		Intents:         []model.ToolIntent{model.ToolIntentStatusRead},
		Risk:            model.RiskLow,
		SideEffect:      effectRead,
	}
}

func (t *AppActionTool) Invoke(ctx context.Context, call model.ToolCall) (model.ToolResult, error) {
	if t.action == nil {
		return model.ToolResult{}, fmt.Errorf("app_action: handler is required")
	}
	var input capabilities.AppActionRequest
	if err := json.Unmarshal(call.InputJSON, &input); err != nil {
		return model.ToolResult{}, fmt.Errorf("app_action: decode input: %w", err)
	}
	if input.Name == "" {
		return model.ToolResult{}, fmt.Errorf("app_action: name is required")
	}
	result, err := t.action(ctx, input)
	if err != nil {
		return model.ToolResult{}, err
	}
	return marshalCapabilityResult("app_action", result)
}

func marshalCapabilityResult(toolName string, payload any) (model.ToolResult, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return model.ToolResult{}, fmt.Errorf("%s: encode output: %w", toolName, err)
	}
	return model.ToolResult{Output: string(raw)}, nil
}
