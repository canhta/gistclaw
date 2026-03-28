import { loadAutomate } from '$lib/automate/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	return {
		automate: await loadAutomate(fetch)
	};
};
