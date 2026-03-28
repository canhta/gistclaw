import { loadHistory } from '$lib/history/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	return {
		history: await loadHistory(fetch)
	};
};
