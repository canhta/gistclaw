import { loadConversations } from '$lib/conversations/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	try {
		const search = url.searchParams.toString();
		const data = await loadConversations(fetch, search);
		return {
			sessions: {
				items: data.sessions ?? [],
				paging: data.paging,
				runtimeConnectors: data.runtime_connectors ?? []
			}
		};
	} catch {
		return {
			sessions: {
				items: [],
				paging: { has_next: false, has_prev: false },
				runtimeConnectors: []
			}
		};
	}
};
