import { requestJSON } from '$lib/http/client';
import type { NodeInventoryResponse } from '$lib/types/api';

export async function loadNodeInventory(fetcher: typeof fetch): Promise<NodeInventoryResponse> {
	return requestJSON<NodeInventoryResponse>(fetcher, '/api/nodes');
}

export function fallbackNodeInventory(
	notice = 'Node inventory could not be loaded. Reload to retry.'
): NodeInventoryResponse {
	return {
		notice,
		summary: {
			connectors: 0,
			healthy_connectors: 0,
			run_nodes: 0,
			approval_nodes: 0,
			capabilities: 0
		},
		connectors: [],
		runs: [],
		capabilities: []
	};
}
