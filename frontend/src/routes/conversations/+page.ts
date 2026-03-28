import { loadConversations } from '$lib/conversations/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	return {
		conversations: await loadConversations(fetch, url.searchParams.toString())
	};
};
