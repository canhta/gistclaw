import { buildConversationListSearch, buildSessionsPageHref } from '$lib/conversations/query';
import { loadConversationDetail } from '$lib/conversations/load';
import { buildHistorySearch } from '$lib/history/query';
import { loadConversations } from '$lib/conversations/load';
import { loadHistory } from '$lib/history/load';
import type { PageLoad } from './$types';

const emptyHistory = {
	summary: {
		run_count: 0,
		completed_runs: 0,
		recovery_runs: 0,
		approval_events: 0,
		delivery_outcomes: 0
	},
	filters: {
		query: '',
		status: '',
		scope: 'all',
		limit: 0
	},
	paging: {
		has_next: false,
		has_prev: false
	},
	runs: [],
	approvals: [],
	deliveries: []
};

export const load: PageLoad = async ({ fetch, url }) => {
	const search = buildConversationListSearch(url.searchParams);
	const historySearch = buildHistorySearch(url.searchParams);
	const historyRequested = url.searchParams.get('tab') === 'history';
	const selectedSessionID = url.searchParams.get('session')?.trim() ?? '';
	const selectedDetailRequested = selectedSessionID !== '';

	try {
		const [data, history, selectedDetail] = await Promise.all([
			loadConversations(fetch, search),
			historyRequested
				? loadHistory(fetch, historySearch).catch(() => emptyHistory)
				: Promise.resolve(emptyHistory),
			selectedDetailRequested
				? loadConversationDetail(fetch, selectedSessionID).catch(() => null)
				: Promise.resolve(null)
		]);
		return {
			sessions: {
				summary: data.summary ?? {
					session_count: 0,
					connector_count: 0,
					terminal_deliveries: 0
				},
				filters: data.filters ?? {
					query: '',
					agent_id: '',
					role: '',
					status: '',
					connector_id: '',
					binding: ''
				},
				items: data.sessions ?? [],
				paging: {
					has_next: data.paging?.has_next ?? false,
					has_prev: data.paging?.has_prev ?? false,
					nextHref: buildSessionsPageHref(data.paging?.next_url, url.searchParams.toString()),
					prevHref: buildSessionsPageHref(data.paging?.prev_url, url.searchParams.toString())
				},
				runtimeConnectors: data.runtime_connectors ?? [],
				selectedDetail,
				history: history ?? emptyHistory
			}
		};
	} catch {
		return {
			sessions: {
				summary: {
					session_count: 0,
					connector_count: 0,
					terminal_deliveries: 0
				},
				filters: {
					query: '',
					agent_id: '',
					role: '',
					status: '',
					connector_id: '',
					binding: ''
				},
				items: [],
				paging: {
					has_next: false,
					has_prev: false,
					nextHref: undefined,
					prevHref: undefined
				},
				runtimeConnectors: [],
				selectedDetail: null,
				history: emptyHistory
			}
		};
	}
};
