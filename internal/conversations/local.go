package conversations

const (
	LocalWebConnectorID       = "web"
	LocalWebAccountID         = "local"
	LocalWebDefaultExternalID = "default"
	LocalDefaultThreadID      = "main"
)

func LocalWebConversationKey(externalID, threadID string) ConversationKey {
	if externalID == "" {
		externalID = LocalWebDefaultExternalID
	}
	if threadID == "" {
		threadID = LocalDefaultThreadID
	}
	return ConversationKey{
		ConnectorID: LocalWebConnectorID,
		AccountID:   LocalWebAccountID,
		ExternalID:  externalID,
		ThreadID:    threadID,
	}
}
