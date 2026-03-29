import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch): Parameters<typeof load>[0] {
	return { fetch: fetcher } as unknown as Parameters<typeof load>[0];
}

describe('instances load', () => {
	it('loads work and conversation signals into presence data', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			if (input === '/api/work') {
				return new Response(
					JSON.stringify({
						active_project_name: 'my-project',
						active_project_path: '/home/user/my-project',
						queue_strip: {
							headline: '1 active run',
							root_runs: 1,
							worker_runs: 1,
							recovery_runs: 0,
							summary: {
								total: 2,
								pending: 0,
								active: 1,
								needs_approval: 1,
								completed: 0,
								failed: 0,
								interrupted: 0,
								root_status: 'active'
							}
						},
						paging: { has_next: false, has_prev: false },
						clusters: [
							{
								root: {
									id: 'run-root',
									objective: 'Review the repo',
									agent_id: 'assistant',
									status: 'active',
									status_label: 'Active',
									status_class: 'is-active',
									model_display: 'gpt-5.4',
									token_summary: '1K tokens',
									started_at_short: '10:00',
									started_at_exact: '2026-03-29 10:00',
									started_at_iso: '2026-03-29T10:00:00Z',
									last_activity_short: '10:05',
									last_activity_exact: '2026-03-29 10:05',
									last_activity_iso: '2026-03-29T10:05:00Z',
									depth: 0
								},
								children: [],
								child_count: 0,
								child_count_label: '0 child runs',
								blocker_label: '',
								has_children: false
							}
						]
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			if (input === '/api/conversations') {
				return new Response(
					JSON.stringify({
						summary: {
							session_count: 2,
							connector_count: 1,
							terminal_deliveries: 0
						},
						filters: {
							query: '',
							agent_id: '',
							role: '',
							status: '',
							connector_id: '',
							binding: ''
						},
						sessions: [],
						paging: { has_next: false, has_prev: false },
						health: [
							{
								connector_id: 'telegram',
								pending_count: 2,
								retrying_count: 0,
								terminal_count: 0,
								state_class: 'is-warning'
							}
						],
						runtime_connectors: [
							{
								connector_id: 'telegram',
								state: 'active',
								state_label: 'Active',
								state_class: 'is-success',
								summary: 'Presence beacons healthy',
								checked_at_label: '1 min ago',
								restart_suggested: false
							}
						]
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			throw new Error(`unexpected request: ${String(input)}`);
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected instances load to return data');
		}

		expect(fetcher).toHaveBeenNthCalledWith(1, '/api/work', expect.any(Object));
		expect(fetcher).toHaveBeenNthCalledWith(2, '/api/conversations', expect.any(Object));
		expect(result.instances.summary).toEqual({
			front_lane_count: 1,
			specialist_lane_count: 0,
			live_connector_count: 1,
			pending_delivery_count: 2
		});
		expect(result.instances.lanes[0]?.agent_id).toBe('assistant');
	});

	it('returns partial fallback data when one source fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			if (input === '/api/work') {
				throw new Error('boom');
			}

			return new Response(
				JSON.stringify({
					summary: {
						session_count: 1,
						connector_count: 1,
						terminal_deliveries: 1
					},
					filters: {
						query: '',
						agent_id: '',
						role: '',
						status: '',
						connector_id: '',
						binding: ''
					},
					sessions: [],
					paging: { has_next: false, has_prev: false },
					health: [
						{
							connector_id: 'telegram',
							pending_count: 1,
							retrying_count: 0,
							terminal_count: 1,
							state_class: 'is-error'
						}
					],
					runtime_connectors: [
						{
							connector_id: 'telegram',
							state: 'degraded',
							state_label: 'Degraded',
							state_class: 'is-error',
							summary: 'Presence beacons stale',
							checked_at_label: '2 min ago',
							restart_suggested: true
						}
					]
				}),
				{
					status: 200,
					headers: { 'content-type': 'application/json' }
				}
			);
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected instances load to return fallback data');
		}

		expect(result.instances.summary).toEqual({
			front_lane_count: 0,
			specialist_lane_count: 0,
			live_connector_count: 0,
			pending_delivery_count: 1
		});
		expect(result.instances.connectors).toEqual([
			expect.objectContaining({
				connector_id: 'telegram',
				terminal_count: 1,
				restart_suggested: true
			})
		]);
	});
});
