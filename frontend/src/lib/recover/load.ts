import { requestJSON } from '$lib/http/client';
import type { RecoverResponse } from '$lib/types/api';

export function loadRecover(fetcher: typeof fetch, search = ''): Promise<RecoverResponse> {
	const suffix = search.trim() === '' ? '' : `?${search}`;
	return requestJSON<RecoverResponse>(fetcher, `/api/recover${suffix}`);
}
