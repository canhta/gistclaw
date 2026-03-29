import { loadSettings } from '$lib/settings/load';
import { loadTeam } from '$lib/team/load';
import { loadWorkIndex } from '$lib/work/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const [settings, team, work] = await Promise.allSettled([
		loadSettings(fetch),
		loadTeam(fetch),
		loadWorkIndex(fetch)
	]);

	return {
		config: {
			settings: settings.status === 'fulfilled' ? settings.value : null,
			team: team.status === 'fulfilled' ? team.value : null,
			work: work.status === 'fulfilled' ? work.value : null
		}
	};
};
