import { loadWorkDetail } from '$lib/work/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => {
	return {
		work: await loadWorkDetail(fetch, params.runId)
	};
};
