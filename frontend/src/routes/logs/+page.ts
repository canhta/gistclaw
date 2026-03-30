import { loadLogs } from '$lib/logs/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	const search = new URLSearchParams();
	const query = url.searchParams.get('q')?.trim() ?? '';
	const level = url.searchParams.get('level')?.trim() ?? 'all';
	const source = url.searchParams.get('source')?.trim() ?? 'all';
	const limit = url.searchParams.get('limit')?.trim() ?? '200';

	if (query !== '') {
		search.set('q', query);
	}
	if (level !== '' && level !== 'all') {
		search.set('level', level);
	}
	if (source !== '' && source !== 'all') {
		search.set('source', source);
	}
	search.set('limit', limit === '' ? '200' : limit);

	try {
		return {
			logs: await loadLogs(fetch, search.toString()),
			logsLoadError: ''
		};
	} catch {
		return {
			logs: null,
			logsLoadError: 'Logs could not be loaded. Reload to retry.'
		};
	}
};
