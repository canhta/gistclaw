import { requestJSON } from '$lib/http/client';

export interface CreateRouteInput {
	sessionID: string;
	connectorID: string;
	externalID: string;
	threadID?: string;
	accountID?: string;
}

interface ConversationRetryDeliveryResponse {
	delivery: {
		id: string;
		status: string;
	};
}

interface RouteCreateResponse {
	route: {
		id: string;
		status: string;
	};
}

interface RouteDeactivateResponse {
	route: {
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

export function deactivateRoute(
	fetcher: typeof fetch,
	routeID: string
): Promise<RouteDeactivateResponse> {
	return requestJSON<RouteDeactivateResponse>(
		fetcher,
		`/api/routes/${encodeURIComponent(routeID)}/deactivate`,
		{
			method: 'POST'
		}
	);
}

export function createRoute(
	fetcher: typeof fetch,
	input: CreateRouteInput
): Promise<RouteCreateResponse> {
	return requestJSON<RouteCreateResponse>(fetcher, '/api/routes', {
		method: 'POST',
		headers: {
			'content-type': 'application/json'
		},
		body: JSON.stringify({
			session_id: input.sessionID,
			connector_id: input.connectorID,
			external_id: input.externalID,
			thread_id: input.threadID,
			account_id: input.accountID
		})
	});
}
