import { loadSettings } from '$lib/settings/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	return {
		settings: await loadSettings(fetch)
	};
};
