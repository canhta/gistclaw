import { loadInstances } from '$lib/instances/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		return {
			instances: await loadInstances(fetch),
			instancesLoadError: ''
		};
	} catch {
		return {
			instances: null,
			instancesLoadError: 'Instance inventory could not be loaded. Reload to retry.'
		};
	}
};
