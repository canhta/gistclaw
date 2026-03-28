import { loadWorkIndex } from '$lib/work/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	return {
		work: await loadWorkIndex(fetch)
	};
};
