package web

import (
	"net/url"
	"strings"
)

const (
	pageLogin      = "/login"
	pageLogout     = "/logout"
	pageOnboarding = "/onboarding"
	pageChat       = "/chat"
	pageChannels   = "/channels"
	pageInstances  = "/instances"
	pageSessions   = "/sessions"
	pageCron       = "/cron"
	pageSkills     = "/skills"
	pageNodes      = "/nodes"
	pageApprovals  = "/approvals"
	pageConfig     = "/config"
	pageDebug      = "/debug"
	pageLogs       = "/logs"
	pageUpdate     = "/update"
)

func runDetailPath(runID string) string {
	return workPagePath(runID)
}

func runGraphPath(runID string) string {
	return workGraphPath(runID)
}

func runEventsPath(runID string) string {
	return workEventsPath(runID)
}

func runEventsPathAfter(runID, after string) string {
	path := runEventsPath(runID)
	if strings.TrimSpace(after) == "" {
		return path
	}
	values := url.Values{}
	values.Set("after", after)
	return path + "?" + values.Encode()
}

func runNodeDetailTemplatePath(runID string) string {
	return workNodeDetailTemplatePath(runID)
}

func workAPIPath(runID string) string {
	return "/api/work/" + url.PathEscape(runID)
}

func workPagePath(_ string) string {
	return pageChat
}

func workGraphPath(runID string) string {
	return workAPIPath(runID) + "/graph"
}

func workEventsPath(runID string) string {
	return workAPIPath(runID) + "/events"
}

func workNodeDetailTemplatePath(runID string) string {
	return workAPIPath(runID) + "/nodes/__RUN_ID__"
}

func workDismissPath(runID, status string) string {
	if status != "interrupted" {
		return ""
	}
	return workAPIPath(runID) + "/dismiss"
}

func sessionDetailPath(_ string) string {
	return pageSessions
}

func sessionMessagePath(sessionID string) string {
	return "/api/conversations/" + url.PathEscape(sessionID) + "/messages"
}

func sessionRetryDeliveryPath(sessionID, deliveryID string) string {
	return "/api/conversations/" + url.PathEscape(sessionID) + "/deliveries/" + url.PathEscape(deliveryID) + "/retry"
}

func approvalResolvePath(approvalID string) string {
	return "/api/recover/approvals/" + url.PathEscape(approvalID) + "/resolve"
}

func routeSendPath(routeID string) string {
	return "/api/recover/routes/" + url.PathEscape(routeID) + "/messages"
}

func routeDeactivatePath(routeID string) string {
	return "/api/recover/routes/" + url.PathEscape(routeID) + "/deactivate"
}

func deliveryRetryPath(deliveryID string) string {
	return "/api/recover/deliveries/" + url.PathEscape(deliveryID) + "/retry"
}
