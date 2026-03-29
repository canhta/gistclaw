import { loadNodeInventory } from '$lib/nodes/load';
import type { NodeInventoryResponse } from '$lib/types/api';
import type { PageLoad } from './$types';

const fallbackNodes: NodeInventoryResponse = {
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

export const load: PageLoad = async ({ fetch }) => {
	try {
		return {
			nodes: await loadNodeInventory(fetch)
		};
	} catch {
		return {
			nodes: fallbackNodes
		};
	}
};
