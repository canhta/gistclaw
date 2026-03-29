import { requestJSON } from '$lib/http/client';

interface ConversationRetryDeliveryResponse {
	delivery: {
		id: string;
		status: string;
	};
}

export function retryConversationDelivery(
	fetcher: typeof fetch,
	sessionID: string,
	deliveryID: string
): Promise<ConversationRetryDeliveryResponse> {
	return requestJSON<ConversationRetryDeliveryResponse>(
		fetcher,
		`/api/conversations/${encodeURIComponent(sessionID)}/deliveries/${encodeURIComponent(deliveryID)}/retry`,
		{
			method: 'POST'
		}
	);
}
