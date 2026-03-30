import { requestJSON } from '$lib/http/client';
import type { NodeInventoryResponse } from '$lib/types/api';

export async function loadNodeInventory(fetcher: typeof fetch): Promise<NodeInventoryResponse> {
	return requestJSON<NodeInventoryResponse>(fetcher, '/api/nodes');
}
