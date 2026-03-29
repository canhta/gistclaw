import { loadAutomate } from '$lib/automate/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const data = await loadAutomate(fetch);
		return {
			cron: {
				summary: data.summary ?? {
					total_schedules: 0,
					enabled_schedules: 0,
					due_schedules: 0,
					active_occurrences: 0,
					next_wake_at_label: 'No wake scheduled'
				},
				health: data.health ?? {
					invalid_schedules: 0,
					stuck_dispatching: 0,
					missing_next_run: 0
				},
				schedules: data.schedules ?? [],
				openOccurrences: data.open_occurrences ?? [],
				recentOccurrences: data.recent_occurrences ?? []
			}
		};
	} catch {
		return {
			cron: {
				summary: {
					total_schedules: 0,
					enabled_schedules: 0,
					due_schedules: 0,
					active_occurrences: 0,
					next_wake_at_label: 'No wake scheduled'
				},
				health: {
					invalid_schedules: 0,
					stuck_dispatching: 0,
					missing_next_run: 0
				},
				schedules: [],
				openOccurrences: [],
				recentOccurrences: []
			}
		};
	}
};
