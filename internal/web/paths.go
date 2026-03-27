package web

import (
	"net/url"
	"strings"
)

const (
	pageLogin                     = "/login"
	pageLogout                    = "/logout"
	pageOperateRuns               = "/operate/runs"
	pageOperateSessions           = "/operate/sessions"
	pageOperateStartTask          = "/operate/start-task"
	pageConfigureTeam             = "/configure/team"
	pageConfigureTeamExport       = "/configure/team/export"
	pageConfigureMemory           = "/configure/memory"
	pageConfigureSettings         = "/configure/settings"
	pageConfigureSettingsPassword = "/configure/settings/password"
	pageConfigureSettingsDevices  = "/configure/settings/devices"
	pageRecoverApprovals          = "/recover/approvals"
	pageRecoverRoutesDeliveries   = "/recover/routes-deliveries"
)

type navLink struct {
	Label  string
	Href   string
	Active bool
}

type shellNavigation struct {
	Groups   []navLink
	Children []navLink
}

func runDetailPath(runID string) string {
	return pageOperateRuns + "/" + url.PathEscape(runID)
}

func runGraphPath(runID string) string {
	return runDetailPath(runID) + "/graph"
}

func runEventsPath(runID string) string {
	return runDetailPath(runID) + "/events"
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

func runNodeDetailPath(runID, nodeRunID string) string {
	return runDetailPath(runID) + "/nodes/" + url.PathEscape(nodeRunID)
}

func runNodeDetailTemplatePath(runID string) string {
	return runDetailPath(runID) + "/nodes/__RUN_ID__"
}

func runDismissPath(runID string) string {
	return runDetailPath(runID) + "/dismiss"
}

func sessionDetailPath(sessionID string) string {
	return pageOperateSessions + "/" + url.PathEscape(sessionID)
}

func sessionMessagePath(sessionID string) string {
	return sessionDetailPath(sessionID) + "/messages"
}

func sessionRetryDeliveryPath(sessionID, deliveryID string) string {
	return sessionDetailPath(sessionID) + "/deliveries/" + url.PathEscape(deliveryID) + "/retry"
}

func approvalResolvePath(approvalID string) string {
	return pageRecoverApprovals + "/" + url.PathEscape(approvalID) + "/resolve"
}

func routeSendPath(routeID string) string {
	return pageRecoverRoutesDeliveries + "/routes/" + url.PathEscape(routeID) + "/messages"
}

func routeDeactivatePath(routeID string) string {
	return pageRecoverRoutesDeliveries + "/routes/" + url.PathEscape(routeID) + "/deactivate"
}

func deliveryRetryPath(deliveryID string) string {
	return pageRecoverRoutesDeliveries + "/deliveries/" + url.PathEscape(deliveryID) + "/retry"
}

func settingsDeviceRevokePath(deviceID string) string {
	return pageConfigureSettingsDevices + "/" + url.PathEscape(deviceID) + "/revoke"
}

func settingsDeviceBlockPath(deviceID string) string {
	return pageConfigureSettingsDevices + "/" + url.PathEscape(deviceID) + "/block"
}

func settingsDeviceUnblockPath(deviceID string) string {
	return pageConfigureSettingsDevices + "/" + url.PathEscape(deviceID) + "/unblock"
}

func navigationForPath(path string) shellNavigation {
	groups := []navLink{
		{Label: "Operate", Href: pageOperateRuns},
		{Label: "Configure", Href: pageConfigureTeam},
		{Label: "Recover", Href: pageRecoverApprovals},
	}

	currentGroup := activeGroup(path)
	for idx := range groups {
		groups[idx].Active = currentGroup != "" && groups[idx].Label == currentGroup
	}

	return shellNavigation{
		Groups:   groups,
		Children: childNavigation(currentGroup, path),
	}
}

func activeGroup(path string) string {
	switch {
	case path == "/operate" || strings.HasPrefix(path, "/operate/"):
		return "Operate"
	case path == "/configure" || strings.HasPrefix(path, "/configure/"):
		return "Configure"
	case path == "/recover" || strings.HasPrefix(path, "/recover/"):
		return "Recover"
	default:
		return ""
	}
}

func childNavigation(group, currentPath string) []navLink {
	var links []navLink
	switch group {
	case "Operate":
		links = []navLink{
			{Label: "Runs", Href: pageOperateRuns},
			{Label: "Sessions", Href: pageOperateSessions},
		}
	case "Configure":
		links = []navLink{
			{Label: "Team", Href: pageConfigureTeam},
			{Label: "Memory", Href: pageConfigureMemory},
			{Label: "Settings", Href: pageConfigureSettings},
		}
	case "Recover":
		links = []navLink{
			{Label: "Approvals", Href: pageRecoverApprovals},
			{Label: "Routes & Deliveries", Href: pageRecoverRoutesDeliveries},
		}
	default:
		return nil
	}

	for idx := range links {
		links[idx].Active = pathMatches(currentPath, links[idx].Href)
	}
	return links
}

func pathMatches(currentPath, href string) bool {
	return currentPath == href || strings.HasPrefix(currentPath, href+"/")
}
