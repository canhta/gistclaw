import { loadAppShell } from '$lib/bootstrap/load';
import type { LayoutLoad } from './$types';

export const ssr = false;

export const load: LayoutLoad = async ({ fetch, url }) => {
	const state = await loadAppShell(fetch);

	return {
		...state,
		currentPath: url.pathname,
		currentSearch: url.search
	};
};
