import { requestJSON } from '$lib/http/client';
import type { HistoryResponse } from '$lib/types/api';

export function loadHistory(fetcher: typeof fetch): Promise<HistoryResponse> {
	return requestJSON<HistoryResponse>(fetcher, '/api/history');
}
