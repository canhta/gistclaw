import { describe, expect, it, vi } from 'vitest';
import { createAutomateSchedule, loadAutomate } from './load';

describe('automate load helpers', () => {
	it('loads automate data from /api/automate', async () => {
		const fetcher = vi.fn<typeof fetch>(
			async () =>
				new Response(JSON.stringify({ schedules: [] }), {
					status: 200,
					headers: { 'content-type': 'application/json' }
				})
		);

		await loadAutomate(fetcher);

		expect(fetcher).toHaveBeenCalledWith('/api/automate', expect.any(Object));
	});

	it('posts schedule creation requests to /api/automate', async () => {
		const fetcher = vi.fn<typeof fetch>(
			async () =>
				new Response(
					JSON.stringify({
						schedule: {
							id: 'sched-1',
							name: 'Daily digest',
							objective: 'Send a daily summary',
							kind: 'cron',
							kind_label: 'Cron',
							cadence_label: 'Cron: 0 9 * * *',
							enabled: true,
							enabled_label: 'Enabled',
							status_label: 'Healthy',
							status_class: 'is-active',
							next_run_at_label: '2026-03-30 02:00 UTC',
							last_run_at_label: 'No executions recorded',
							last_error: '',
							project_id: 'p1',
							cwd: '/home/user/project',
							consecutive_failures: 0,
							schedule_error_count: 0
						}
					}),
					{
						status: 201,
						headers: { 'content-type': 'application/json' }
					}
				)
		);

		await createAutomateSchedule(fetcher, {
			name: 'Daily digest',
			objective: 'Send a daily summary',
			kind: 'cron',
			cron_expr: '0 9 * * *',
			timezone: 'UTC'
		});

		expect(fetcher).toHaveBeenCalledWith('/api/automate', {
			method: 'POST',
			body: JSON.stringify({
				name: 'Daily digest',
				objective: 'Send a daily summary',
				kind: 'cron',
				cron_expr: '0 9 * * *',
				timezone: 'UTC'
			}),
			headers: {
				accept: 'application/json',
				'content-type': 'application/json'
			}
		});
	});
});
