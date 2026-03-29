import { loadConversations } from '$lib/conversations/load';
import { buildInstancePresenceData } from '$lib/instances/presence';
import { loadWorkIndex } from '$lib/work/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const [work, conversations] = await Promise.allSettled([
		loadWorkIndex(fetch),
		loadConversations(fetch)
	]);

	return {
		instances: buildInstancePresenceData(
			work.status === 'fulfilled' ? work.value : null,
			conversations.status === 'fulfilled' ? conversations.value : null
		)
	};
};
