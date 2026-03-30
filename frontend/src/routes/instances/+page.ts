import { fallbackInstances, loadInstances } from '$lib/instances/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		return {
			instances: await loadInstances(fetch)
		};
	} catch {
		return {
			instances: fallbackInstances()
		};
	}
};
