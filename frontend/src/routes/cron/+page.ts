import { loadAutomate } from '$lib/automate/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const data = await loadAutomate(fetch);
		return {
			cron: {
				schedules: data.schedules ?? [],
				occurrences: [...(data.open_occurrences ?? []), ...(data.recent_occurrences ?? [])]
			}
		};
	} catch {
		return {
			cron: {
				schedules: [],
				occurrences: []
			}
		};
	}
};
