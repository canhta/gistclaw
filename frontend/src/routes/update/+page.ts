import { fallbackUpdateStatus, loadUpdateStatus } from '$lib/update/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		return {
			update: await loadUpdateStatus(fetch)
		};
	} catch {
		return {
			update: fallbackUpdateStatus()
		};
	}
};
