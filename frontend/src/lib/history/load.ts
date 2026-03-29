import { requestJSON } from '$lib/http/client';
import type { HistoryResponse } from '$lib/types/api';

export function loadHistory(fetcher: typeof fetch, search = ''): Promise<HistoryResponse> {
	const suffix = search.trim();
	return requestJSON<HistoryResponse>(
		fetcher,
		suffix === '' ? '/api/history' : `/api/history?${suffix}`
	);
}
