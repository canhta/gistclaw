import { loadTeam } from '$lib/team/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	return {
		team: await loadTeam(fetch)
	};
};
