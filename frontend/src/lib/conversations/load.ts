import { requestJSON } from '$lib/http/client';
import { buildSessionsDeliveryHref } from '$lib/conversations/query';
import type {
	ConversationDeliveryQueueItemResponse,
	ConversationDeliveryQueueResponse,
	ConversationDetailResponse,
	ConversationsResponse
} from '$lib/types/api';

interface DeliveryQueueItemRaw {
	ID?: string;
	RunID?: string;
	SessionID?: string;
	ConversationID?: string;
	ConnectorID?: string;
	ChatID?: string;
	MessageText?: string;
	Status?: string;
	Attempts?: number;
}

interface DeliveryQueueRawResponse {
	deliveries?: DeliveryQueueItemRaw[];
	next_cursor?: string;
	prev_cursor?: string;
	has_next?: boolean;
	has_prev?: boolean;
}

export function loadConversations(
	fetcher: typeof fetch,
	search = ''
): Promise<ConversationsResponse> {
	const suffix = search.trim() === '' ? '' : `?${search}`;
	return requestJSON<ConversationsResponse>(fetcher, `/api/conversations${suffix}`);
}

export function loadConversationDetail(
	fetcher: typeof fetch,
	sessionID: string
): Promise<ConversationDetailResponse> {
	return requestJSON<ConversationDetailResponse>(
		fetcher,
		`/api/conversations/${encodeURIComponent(sessionID)}`
	);
}

export async function loadConversationDeliveryQueue(
	fetcher: typeof fetch,
	search = '',
	currentSearch = ''
): Promise<ConversationDeliveryQueueResponse> {
	const suffix = search.trim() === '' ? '' : `?${search}`;
	const raw = await requestJSON<DeliveryQueueRawResponse>(fetcher, `/api/deliveries${suffix}`);
	const params = new URLSearchParams(search);
	const sessionID = params.get('session_id')?.trim() ?? '';
	const limit = params.get('limit')?.trim() ?? '50';

	return {
		items: (raw.deliveries ?? []).map(normalizeConversationDeliveryQueueItem),
		paging: {
			has_next: raw.has_next ?? false,
			has_prev: raw.has_prev ?? false,
			nextHref: buildSessionsDeliveryHref(raw.next_cursor, 'next', sessionID, currentSearch, limit),
			prevHref: buildSessionsDeliveryHref(raw.prev_cursor, 'prev', sessionID, currentSearch, limit)
		}
	};
}

function normalizeConversationDeliveryQueueItem(
	item: DeliveryQueueItemRaw
): ConversationDeliveryQueueItemResponse {
	return {
		id: item.ID ?? '',
		run_id: item.RunID ?? '',
		session_id: item.SessionID ?? '',
		conversation_id: item.ConversationID ?? '',
		connector_id: item.ConnectorID ?? '',
		chat_id: item.ChatID ?? '',
		status: item.Status ?? '',
		status_label: humanizeLabel(item.Status ?? ''),
		attempts_label: formatAttempts(item.Attempts ?? 0),
		message_preview: item.MessageText ?? ''
	};
}

function humanizeLabel(value: string): string {
	return value
		.trim()
		.split(/[_-]+/)
		.filter((part) => part !== '')
		.map((part) => part.charAt(0).toUpperCase() + part.slice(1))
		.join(' ');
}

function formatAttempts(value: number): string {
	return value === 1 ? '1 attempt' : `${value} attempts`;
}
