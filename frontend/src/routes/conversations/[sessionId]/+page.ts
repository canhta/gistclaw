import { loadConversationDetail } from '$lib/conversations/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => {
	return {
		conversation: await loadConversationDetail(fetch, params.sessionId)
	};
};
