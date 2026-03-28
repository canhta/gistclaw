import { requestJSON } from '$lib/http/client';
import type { ConversationDetailResponse, ConversationsResponse } from '$lib/types/api';

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
