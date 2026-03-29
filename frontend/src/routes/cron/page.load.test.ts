import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch): Parameters<typeof load>[0] {
	return { fetch: fetcher } as unknown as Parameters<typeof load>[0];
}

describe('cron load', () => {
	it('loads schedules and occurrences from automate', async () => {
		const fetcher = vi.fn<typeof fetch>(
			async () =>
				new Response(
					JSON.stringify({
						summary: {
							total_schedules: 1,
							enabled_schedules: 1,
							due_schedules: 0,
							active_occurrences: 1,
							next_wake_at_label: 'in 3h'
						},
						health: {
							invalid_schedules: 0,
							stuck_dispatching: 0,
							missing_next_run: 0
						},
						schedules: [
							{
								id: 'sched-1',
								name: 'Daily digest',
								objective: 'Send a daily summary',
								kind: 'cron',
								kind_label: 'Cron',
								cadence_label: 'Every day at 09:00',
								enabled: true,
								enabled_label: 'Enabled',
								status_label: 'OK',
								status_class: 'is-success',
								next_run_at_label: 'in 3h',
								last_run_at_label: '1 day ago',
								last_error: '',
								project_id: 'p1',
								cwd: '/home/user/project',
								consecutive_failures: 0,
								schedule_error_count: 0
							}
						],
						open_occurrences: [
							{
								id: 'occ-1',
								schedule_id: 'sched-1',
								schedule_name: 'Daily digest',
								status: 'running',
								status_label: 'Running',
								status_class: 'is-active',
								slot_at_label: 'Today 09:00',
								updated_at_label: 'just now'
							}
						],
						recent_occurrences: []
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				)
		);

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected cron load to return data');
		}

		expect(fetcher).toHaveBeenCalledWith('/api/automate', expect.any(Object));
		expect(result.cron.schedules).toHaveLength(1);
		expect(result.cron.occurrences).toHaveLength(1);
	});

	it('returns empty cron data when the request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected cron load to return fallback data');
		}

		expect(result).toEqual({
			cron: {
				schedules: [],
				occurrences: []
			}
		});
	});
});
