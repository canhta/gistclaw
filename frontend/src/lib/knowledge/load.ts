import { requestJSON } from '$lib/http/client';
import type { KnowledgeResponse } from '$lib/types/api';

export function loadKnowledge(fetcher: typeof fetch, search = ''): Promise<KnowledgeResponse> {
	const suffix = search.trim() === '' ? '' : `?${search}`;
	return requestJSON<KnowledgeResponse>(fetcher, `/api/knowledge${suffix}`);
}
