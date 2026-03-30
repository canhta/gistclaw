import { loadNodeInventory } from '$lib/nodes/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		return {
			nodes: await loadNodeInventory(fetch),
			nodesLoadError: ''
		};
	} catch {
		return {
			nodes: null,
			nodesLoadError: 'Node inventory could not be loaded. Reload to retry.'
		};
	}
};
