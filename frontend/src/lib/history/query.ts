const historyFilterKeyMap = {
	history_q: 'q',
	history_status: 'status',
	history_scope: 'scope',
	history_limit: 'limit'
} as const;

export function buildHistorySearch(params: URLSearchParams): string {
	const next = new URLSearchParams();

	for (const [pageKey, apiKey] of Object.entries(historyFilterKeyMap)) {
		const value = params.get(pageKey);
		if (value != null && value.trim() !== '') {
			next.set(apiKey, value);
		}
	}

	return next.toString();
}
