import { loadUpdateStatus } from '$lib/update/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		return {
			update: await loadUpdateStatus(fetch),
			updateLoadError: ''
		};
	} catch {
		return {
			update: null,
			updateLoadError: 'Update status could not be loaded. Reload to retry.'
		};
	}
};
