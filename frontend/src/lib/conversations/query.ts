const conversationListKeys = [
	'q',
	'agent_id',
	'role',
	'status',
	'connector_id',
	'binding',
	'cursor',
	'direction',
	'limit'
] as const;

const sessionDeliveryKeyMap = {
	delivery_q: 'q',
	delivery_status: 'status',
	delivery_cursor: 'cursor',
	delivery_direction: 'direction',
	delivery_limit: 'limit'
} as const;

export function buildConversationListSearch(params: URLSearchParams): string {
	const next = new URLSearchParams();

	for (const key of conversationListKeys) {
		const value = params.get(key);
		if (value != null && value.trim() !== '') {
			next.set(key, value);
		}
	}

	return next.toString();
}

export function buildSessionDeliverySearch(params: URLSearchParams, sessionID: string): string {
	const next = new URLSearchParams();
	const normalizedSessionID = sessionID.trim();
	if (normalizedSessionID === '') {
		return '';
	}

	next.set('session_id', normalizedSessionID);

	for (const [sourceKey, targetKey] of Object.entries(sessionDeliveryKeyMap)) {
		const value = params.get(sourceKey);
		if (value != null && value.trim() !== '') {
			next.set(targetKey, value);
		}
	}

	if (!next.has('limit')) {
		next.set('limit', '50');
	}

	return next.toString();
}

export function buildSessionsPageHref(
	apiHref: string | undefined,
	currentSearch = ''
): string | undefined {
	if (apiHref == null || apiHref.trim() === '') {
		return undefined;
	}

	const url = new URL(apiHref, 'http://localhost');
	const next = new URLSearchParams(url.search);
	const current = new URLSearchParams(currentSearch);
	const tab = current.get('tab');

	if (tab != null && tab.trim() !== '') {
		next.set('tab', tab);
	}

	const suffix = next.toString();
	return suffix === '' ? '/sessions' : `/sessions?${suffix}`;
}

export function buildSessionsDeliveryHref(
	cursor: string | undefined,
	direction: 'next' | 'prev',
	sessionID: string,
	currentSearch = '',
	limit?: string
): string | undefined {
	if (cursor == null || cursor.trim() === '' || sessionID.trim() === '') {
		return undefined;
	}

	const next = new URLSearchParams(currentSearch);
	next.set('tab', 'overrides');
	next.set('session', sessionID);
	next.set('delivery_cursor', cursor);
	next.set('delivery_direction', direction);

	const resolvedLimit = limit?.trim() || next.get('delivery_limit')?.trim() || '50';
	next.set('delivery_limit', resolvedLimit);

	const suffix = next.toString();
	return suffix === '' ? '/sessions' : `/sessions?${suffix}`;
}
