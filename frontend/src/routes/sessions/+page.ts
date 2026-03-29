import { buildConversationListSearch, buildSessionsPageHref } from '$lib/conversations/query';
import { loadConversations } from '$lib/conversations/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	const search = buildConversationListSearch(url.searchParams);

	try {
		const data = await loadConversations(fetch, search);
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
				runtimeConnectors: data.runtime_connectors ?? []
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
				runtimeConnectors: []
			}
		};
	}
};
