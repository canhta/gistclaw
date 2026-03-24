package sessions

func BuildFrontSessionKey(conversationID string) string {
	return "front:" + conversationID
}

func BuildWorkerSessionKey(parentSessionID, agentID string) string {
	return "worker:" + parentSessionID + ":" + agentID
}
