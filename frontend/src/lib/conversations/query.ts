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
