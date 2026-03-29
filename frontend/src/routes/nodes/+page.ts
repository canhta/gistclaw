import { fallbackNodeInventory, loadNodeInventory } from '$lib/nodes/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		return {
			nodes: await loadNodeInventory(fetch)
		};
	} catch {
		return {
			nodes: fallbackNodeInventory()
		};
	}
};
