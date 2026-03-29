import { requestJSON } from '$lib/http/client';
import type { LogsResponse } from '$lib/types/api';

export async function loadLogs(fetcher: typeof fetch, search = ''): Promise<LogsResponse> {
	const suffix = search.trim();
	return requestJSON<LogsResponse>(fetcher, suffix === '' ? '/api/logs' : `/api/logs?${suffix}`);
}
