import { loadConversations } from '$lib/conversations/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const data = await loadConversations(fetch);
		return {
			channels: {
				connectors: data.runtime_connectors ?? [],
				deliveryHealth: data.health ?? []
			}
		};
	} catch {
		return {
			channels: {
				connectors: [],
				deliveryHealth: []
			}
		};
	}
};
