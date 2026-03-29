import { buildChannelRoutesSearch, loadChannelRoutes } from '$lib/channels/routes';
import { buildChannelStatusData } from '$lib/channels/status';
import { loadConversations } from '$lib/conversations/load';
import type { PageLoad } from './$types';

const emptyRoutes = {
	filters: {
		connector_id: '',
		status: '',
		query: '',
		limit: 50
	},
	items: [],
	paging: {
		has_next: false,
		has_prev: false,
		nextHref: undefined,
		prevHref: undefined
	}
};

export const load: PageLoad = async ({ fetch, url }) => {
	try {
		const settingsRequested = url.searchParams.get('tab') === 'settings';
		const currentSearch = url.searchParams.toString();
		const [data, routes] = await Promise.all([
			loadConversations(fetch),
			settingsRequested
				? loadChannelRoutes(fetch, buildChannelRoutesSearch(url.searchParams), currentSearch).catch(
						() => emptyRoutes
					)
				: Promise.resolve(emptyRoutes)
		]);
		const channels = buildChannelStatusData(data.runtime_connectors ?? [], data.health ?? []);
		return {
			channels: {
				...channels,
				routes
			}
		};
	} catch {
		return {
			channels: {
				summary: {
					connector_count: 0,
					active_count: 0,
					pending_count: 0,
					retrying_count: 0,
					terminal_count: 0,
					restart_suggested_count: 0
				},
				items: [],
				routes: emptyRoutes
			}
		};
	}
};
