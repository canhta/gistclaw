import { requestJSON } from '$lib/http/client';
import type { KnowledgeItemResponse } from '$lib/types/api';

interface KnowledgeForgetResponse {
	id: string;
	forgotten: boolean;
}

export function editKnowledgeItem(
	fetcher: typeof fetch,
	itemID: string,
	content: string
): Promise<KnowledgeItemResponse> {
	return requestJSON<KnowledgeItemResponse>(
		fetcher,
		`/api/knowledge/${encodeURIComponent(itemID)}/edit`,
		{
			method: 'POST',
			headers: {
				'content-type': 'application/json'
			},
			body: JSON.stringify({
				content
			})
		}
	);
}

export function forgetKnowledgeItem(
	fetcher: typeof fetch,
	itemID: string
): Promise<KnowledgeForgetResponse> {
	return requestJSON<KnowledgeForgetResponse>(
		fetcher,
		`/api/knowledge/${encodeURIComponent(itemID)}/forget`,
		{
			method: 'POST'
		}
	);
}
