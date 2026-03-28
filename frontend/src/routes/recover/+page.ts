import { loadRecover } from '$lib/recover/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	return {
		recover: await loadRecover(fetch, url.searchParams.toString())
	};
};
