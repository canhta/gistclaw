import { redirect } from '@sveltejs/kit';
import { loadAppShell } from '$lib/bootstrap/load';
import type { LayoutLoad } from './$types';

export const ssr = false;

export const load: LayoutLoad = async ({ fetch, url }) => {
	const state = await loadAppShell(fetch);
	if (!state.auth.authenticated && url.pathname !== '/login') {
		throw redirect(307, loginHref(url));
	}

	return {
		...state,
		currentPath: url.pathname,
		currentSearch: url.search
	};
};

function loginHref(url: URL): string {
	const next = `${url.pathname}${url.search}`.trim();
	if (next === '' || next === '/login') {
		return '/login';
	}
	return `/login?next=${encodeURIComponent(next)}`;
}
