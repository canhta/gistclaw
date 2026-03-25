package sessions

func BuildFrontSessionKey(conversationID string) string {
	return "front:" + conversationID
}

func BuildWorkerSessionKey(parentSessionID, agentID, sessionID string) string {
	return "worker:" + parentSessionID + ":" + agentID + ":" + sessionID
}
