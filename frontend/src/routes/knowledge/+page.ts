import { loadKnowledge } from '$lib/knowledge/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	return {
		knowledge: await loadKnowledge(fetch, url.searchParams.toString())
	};
};
