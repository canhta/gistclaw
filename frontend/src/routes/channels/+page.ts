import { buildChannelStatusData } from '$lib/channels/status';
import { loadConversations } from '$lib/conversations/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const data = await loadConversations(fetch);
		const channels = buildChannelStatusData(data.runtime_connectors ?? [], data.health ?? []);
		return {
			channels
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
				items: []
			}
		};
	}
};
