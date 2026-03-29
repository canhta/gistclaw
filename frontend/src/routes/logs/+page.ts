import { loadLogs } from '$lib/logs/load';
import type { PageLoad } from './$types';

const emptyLogs = {
	summary: {
		buffered_entries: 0,
		visible_entries: 0,
		error_entries: 0,
		warning_entries: 0
	},
	filters: {
		query: '',
		level: 'all',
		source: 'all',
		limit: 200
	},
	sources: [],
	stream_url: '/api/logs/stream?limit=200',
	entries: []
};

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
			logs: await loadLogs(fetch, search.toString())
		};
	} catch {
		return {
			logs: emptyLogs
		};
	}
};
