package providerutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/canhta/gistclaw/internal/model"
)

type ToolCallRecordedPayload struct {
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	InputJSON  json.RawMessage `json:"input_json"`
	OutputJSON json.RawMessage `json:"output_json"`
	Decision   string          `json:"decision"`
}

const maxToolResultContentRunes = 8 << 10

const toolResultTruncationNotice = "\n\n[tool result truncated for model context; rerun with a narrower scope or inspect the source directly]"

func RenderToolResultContent(raw json.RawMessage) string {
	content := ""
	var result model.ToolResult
	if err := json.Unmarshal(raw, &result); err == nil {
		switch {
		case result.Output != "" && result.Error != "":
			content = result.Output + "\n" + result.Error
		case result.Output != "":
			content = result.Output
		case result.Error != "":
			content = result.Error
		}
	}
	if content == "" {
		if len(raw) == 0 {
			return ""
		}
		content = string(raw)
	}
	return truncateToolResultContent(content)
}

func truncateToolResultContent(content string) string {
	runes := []rune(content)
	if len(runes) <= maxToolResultContentRunes {
		return content
	}

	notice := []rune(toolResultTruncationNotice)
	limit := maxToolResultContentRunes - len(notice)
	if limit <= 0 {
		return string(notice)
	}
	return string(runes[:limit]) + toolResultTruncationNotice
}

func SchemaObject(raw string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	if strings.TrimSpace(raw) == "" {
		return schema
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil || parsed == nil {
		return schema
	}

	if typeName, ok := parsed["type"].(string); !ok || typeName == "" {
		parsed["type"] = "object"
	}
	if parsed["type"] == "object" {
		if _, ok := parsed["properties"]; !ok {
			parsed["properties"] = map[string]any{}
		}
	}
	return parsed
}

func ProviderError(provider string, err error) error {
	statusCode, raw := sdkErrorDetails(err)
	if raw == "" {
		return fmt.Errorf("%s: %w", provider, err)
	}

	var payload struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Error   struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if unmarshalErr := json.Unmarshal([]byte(raw), &payload); unmarshalErr != nil {
		return &model.ProviderError{
			Code:      model.ErrMalformedResponse,
			Message:   err.Error(),
			Retryable: statusCode == http.StatusTooManyRequests || statusCode >= 500,
		}
	}

	if payload.Error.Type != "" {
		payload.Type = payload.Error.Type
		payload.Message = payload.Error.Message
	}

	errCode := model.ProviderErrorCode(payload.Type)
	if errCode == "" {
		errCode = model.ErrMalformedResponse
	}
	message := payload.Message
	if message == "" {
		message = err.Error()
	}

	return &model.ProviderError{
		Code:      errCode,
		Message:   message,
		Retryable: statusCode == http.StatusTooManyRequests || statusCode >= 500,
	}
}

func sdkErrorDetails(err error) (int, string) {
	type rawJSONer interface {
		RawJSON() string
	}

	statusCode := 0
	raw := ""

	if rj, ok := err.(rawJSONer); ok {
		raw = rj.RawJSON()
	}

	v := reflect.ValueOf(err)
	if v.IsValid() && v.Kind() == reflect.Ptr && !v.IsNil() {
		elem := v.Elem()
		if elem.IsValid() && elem.Kind() == reflect.Struct {
			field := elem.FieldByName("StatusCode")
			if field.IsValid() && field.CanInt() {
				statusCode = int(field.Int())
			}
		}
	}

	return statusCode, raw
}
