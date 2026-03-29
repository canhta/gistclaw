import { loadWorkIndex } from '$lib/work/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const index = await loadWorkIndex(fetch);
		return {
			chat: {
				runs: index.clusters,
				paging: index.paging
			}
		};
	} catch {
		return {
			chat: {
				runs: [],
				paging: { has_next: false, has_prev: false }
			}
		};
	}
};
