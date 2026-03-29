import { buildChatPageHref, buildWorkListSearch } from '$lib/work/query';
import { loadWorkDetail, loadWorkIndex } from '$lib/work/load';
import type { PageLoad } from './$types';

const emptyQueue = {
	headline: 'No active runs',
	root_runs: 0,
	worker_runs: 0,
	recovery_runs: 0,
	summary: {
		total: 0,
		pending: 0,
		active: 0,
		needs_approval: 0,
		completed: 0,
		failed: 0,
		interrupted: 0,
		root_status: 'idle'
	}
};

export const load: PageLoad = async ({ fetch, url }) => {
	const search = buildWorkListSearch(url.searchParams);

	try {
		const index = await loadWorkIndex(fetch, search);
		const requestedRunID = url.searchParams.get('run')?.trim() ?? '';
		const selectedRunID = requestedRunID || index.clusters[0]?.root.id || null;

		let detail = null;
		if (selectedRunID) {
			try {
				detail = await loadWorkDetail(fetch, selectedRunID);
			} catch {
				detail = null;
			}
		}

		return {
			chat: {
				projectName: index.active_project_name ?? '',
				projectPath: index.active_project_path ?? '',
				queue: index.queue_strip ?? emptyQueue,
				runs: index.clusters,
				paging: {
					has_next: index.paging?.has_next ?? false,
					has_prev: index.paging?.has_prev ?? false,
					nextHref: buildChatPageHref(index.paging?.next_url, url.searchParams.toString()),
					prevHref: buildChatPageHref(index.paging?.prev_url, url.searchParams.toString())
				},
				selectedRunID,
				detail
			}
		};
	} catch {
		return {
			chat: {
				projectName: '',
				projectPath: '',
				queue: emptyQueue,
				runs: [],
				paging: { has_next: false, has_prev: false, nextHref: undefined, prevHref: undefined },
				selectedRunID: null,
				detail: null
			}
		};
	}
};
