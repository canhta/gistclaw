package i18n

import "strings"

type Locale string

const (
	LocaleEnglish    Locale = "en"
	LocaleVietnamese Locale = "vi"
)

type MessageID string

const (
	MessageApprovalPromptTitle         MessageID = "approval.prompt.title"
	MessageApprovalPromptTitleWithTool MessageID = "approval.prompt.title_with_tool"
	MessageApprovalBlockedAction       MessageID = "approval.prompt.blocked_action"
	MessageApprovalReason              MessageID = "approval.prompt.reason"
	MessageApprovalReplyInstruction    MessageID = "approval.prompt.reply_instruction"
	MessageApprovalCommandFallback     MessageID = "approval.prompt.command_fallback"
	MessageApprovalButtonApprove       MessageID = "approval.button.approve"
	MessageApprovalButtonDeny          MessageID = "approval.button.deny"
	MessageApprovalResolvedApproved    MessageID = "approval.resolved.approved"
	MessageApprovalResolvedDenied      MessageID = "approval.resolved.denied"
	MessageApprovalResolvedUpdated     MessageID = "approval.resolved.updated"
	MessageGateClarificationDefault    MessageID = "gate.clarification.default"
	MessageGateCommandUsage            MessageID = "gate.command.usage"
	MessageGateCommandMismatch         MessageID = "gate.command.mismatch"
	MessageControlHelpIntro            MessageID = "control.help.intro"
	MessageControlHelpHeader           MessageID = "control.help.header"
	MessageControlHelpCommandHelp      MessageID = "control.help.command.help"
	MessageControlHelpCommandStatus    MessageID = "control.help.command.status"
	MessageControlHelpCommandReset     MessageID = "control.help.command.reset"
	MessageControlStatusNoRuns         MessageID = "control.status.no_runs"
	MessageControlStatusNoRunsHint     MessageID = "control.status.no_runs_hint"
	MessageControlStatusActiveRun      MessageID = "control.status.active_run"
	MessageControlStatusNoActiveRun    MessageID = "control.status.no_active_run"
	MessageControlStatusLastRun        MessageID = "control.status.last_run"
	MessageControlStatusActiveGate     MessageID = "control.status.active_gate"
	MessageControlStatusPendingOne     MessageID = "control.status.pending_one"
	MessageControlStatusPendingMany    MessageID = "control.status.pending_many"
	MessageControlStatusNoObjective    MessageID = "control.status.no_objective"
	MessageControlResetMissing         MessageID = "control.reset.missing"
	MessageControlResetBusy            MessageID = "control.reset.busy"
	MessageControlResetCleared         MessageID = "control.reset.cleared"
)

type Catalog map[Locale]map[MessageID]string

var DefaultCatalog = Catalog{
	LocaleEnglish: {
		MessageApprovalPromptTitle:         "Approval required",
		MessageApprovalPromptTitleWithTool: "Approval required for {tool_name}",
		MessageApprovalBlockedAction:       "Blocked action: {summary}.",
		MessageApprovalReason:              "{reason}",
		MessageApprovalReplyInstruction:    "Reply naturally in any language to approve or deny it here.",
		MessageApprovalCommandFallback:     "Command fallback: /approve {approval_id} allow-once or /approve {approval_id} deny",
		MessageApprovalButtonApprove:       "Approve",
		MessageApprovalButtonDeny:          "Deny",
		MessageApprovalResolvedApproved:    "Approved here in chat. Continuing the task.",
		MessageApprovalResolvedDenied:      "Denied here in chat. The blocked action will be skipped.",
		MessageApprovalResolvedUpdated:     "Updated the pending approval.",
		MessageGateClarificationDefault:    "I couldn't tell whether you want to approve or deny that. Reply yes/no, approve/deny, or answer in your language.",
		MessageGateCommandUsage:            "Usage: /approve {approval_id} allow-once|allow-always|deny",
		MessageGateCommandMismatch:         "That approval ID does not match the pending request.",
		MessageControlHelpIntro:            "Message me naturally to start a task.",
		MessageControlHelpHeader:           "Native commands:",
		MessageControlHelpCommandHelp:      "/help   Show this help",
		MessageControlHelpCommandStatus:    "/status Show the latest status for this chat",
		MessageControlHelpCommandReset:     "/reset  Clear this chat's history and temp state",
		MessageControlStatusNoRuns:         "No runs yet for this chat.",
		MessageControlStatusNoRunsHint:     "Message me naturally to start one.",
		MessageControlStatusActiveRun:      "Active run {run_id} is working on: {objective}",
		MessageControlStatusNoActiveRun:    "No active run for this chat.",
		MessageControlStatusLastRun:        "Last run {run_id} finished with status {status}: {objective}",
		MessageControlStatusActiveGate:     "Waiting for your reply: {title}",
		MessageControlStatusPendingOne:     "1 pending decision is waiting for a reply in this chat.",
		MessageControlStatusPendingMany:    "{count} pending decisions are waiting for replies in this chat.",
		MessageControlStatusNoObjective:    "no objective recorded",
		MessageControlResetMissing:         "Nothing to reset for this chat.",
		MessageControlResetBusy:            "This chat has an active run right now. Retry /reset in a moment.",
		MessageControlResetCleared:         "Chat reset. History cleared for this chat.",
	},
	LocaleVietnamese: {
		MessageApprovalPromptTitle:         "Cần phê duyệt",
		MessageApprovalPromptTitleWithTool: "Cần phê duyệt cho {tool_name}",
		MessageApprovalBlockedAction:       "Hành động đang bị chặn: {summary}.",
		MessageApprovalReason:              "{reason}",
		MessageApprovalReplyInstruction:    "Bạn có thể trả lời tự nhiên bằng bất kỳ ngôn ngữ nào để phê duyệt hoặc từ chối ngay tại đây.",
		MessageApprovalCommandFallback:     "Lệnh dự phòng: /approve {approval_id} allow-once hoặc /approve {approval_id} deny",
		MessageApprovalButtonApprove:       "Phê duyệt",
		MessageApprovalButtonDeny:          "Từ chối",
		MessageApprovalResolvedApproved:    "Đã phê duyệt ngay trong chat. Đang tiếp tục tác vụ.",
		MessageApprovalResolvedDenied:      "Đã từ chối ngay trong chat. Hành động bị chặn sẽ được bỏ qua.",
		MessageApprovalResolvedUpdated:     "Đã cập nhật yêu cầu phê duyệt đang chờ.",
		MessageGateClarificationDefault:    "Mình chưa xác định được bạn muốn phê duyệt hay từ chối. Hãy trả lời có/không, approve/deny, hoặc trả lời bằng ngôn ngữ của bạn.",
		MessageGateCommandUsage:            "Cú pháp: /approve {approval_id} allow-once|allow-always|deny",
		MessageGateCommandMismatch:         "Mã phê duyệt này không khớp với yêu cầu đang chờ.",
		MessageControlHelpIntro:            "Nhắn cho mình tự nhiên để bắt đầu một tác vụ.",
		MessageControlHelpHeader:           "Lệnh hỗ trợ:",
		MessageControlHelpCommandHelp:      "/help   Xem trợ giúp này",
		MessageControlHelpCommandStatus:    "/status Xem trạng thái mới nhất của chat này",
		MessageControlHelpCommandReset:     "/reset  Xóa lịch sử và trạng thái tạm của chat này",
		MessageControlStatusNoRuns:         "Chat này chưa có tác vụ nào.",
		MessageControlStatusNoRunsHint:     "Nhắn cho mình tự nhiên để bắt đầu một tác vụ.",
		MessageControlStatusActiveRun:      "Tiến trình đang chạy {run_id} đang xử lý: {objective}",
		MessageControlStatusNoActiveRun:    "Chat này hiện không có tiến trình đang chạy.",
		MessageControlStatusLastRun:        "Tiến trình gần nhất {run_id} kết thúc với trạng thái {status}: {objective}",
		MessageControlStatusActiveGate:     "Đang chờ bạn trả lời: {title}",
		MessageControlStatusPendingOne:     "Có 1 quyết định đang chờ phản hồi trong chat này.",
		MessageControlStatusPendingMany:    "Có {count} quyết định đang chờ phản hồi trong chat này.",
		MessageControlStatusNoObjective:    "chưa có mục tiêu nào được ghi lại",
		MessageControlResetMissing:         "Chat này không có gì để đặt lại.",
		MessageControlResetBusy:            "Chat này đang có một tiến trình hoạt động. Hãy thử /reset lại sau.",
		MessageControlResetCleared:         "Đã đặt lại chat. Lịch sử của chat này đã được xóa.",
	},
}

func ResolveLocale(hint string) Locale {
	trimmed := strings.TrimSpace(strings.ToLower(hint))
	if trimmed == "" {
		return LocaleEnglish
	}
	base := trimmed
	if idx := strings.IndexAny(base, "-_"); idx >= 0 {
		base = base[:idx]
	}
	switch Locale(base) {
	case LocaleVietnamese:
		return LocaleVietnamese
	case LocaleEnglish:
		return LocaleEnglish
	default:
		return LocaleEnglish
	}
}

func (c Catalog) Format(hint string, id MessageID, values map[string]string) string {
	locale := ResolveLocale(hint)
	if msg := c.lookup(locale, id); msg != "" {
		return replacePlaceholders(msg, values)
	}
	if msg := c.lookup(LocaleEnglish, id); msg != "" {
		return replacePlaceholders(msg, values)
	}
	return string(id)
}

func (c Catalog) lookup(locale Locale, id MessageID) string {
	messages := c[locale]
	if len(messages) == 0 {
		return ""
	}
	return messages[id]
}

func replacePlaceholders(template string, values map[string]string) string {
	if len(values) == 0 {
		return template
	}
	replacements := make([]string, 0, len(values)*2)
	for key, value := range values {
		replacements = append(replacements, "{"+key+"}", value)
	}
	return strings.NewReplacer(replacements...).Replace(template)
}
