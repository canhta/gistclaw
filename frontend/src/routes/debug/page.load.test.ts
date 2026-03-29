import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch): Parameters<typeof load>[0] {
	return { fetch: fetcher } as unknown as Parameters<typeof load>[0];
}

describe('debug load', () => {
	it('loads settings, work queue, and delivery health in parallel', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			switch (String(input)) {
				case '/api/settings':
					return new Response(
						JSON.stringify({
							machine: {
								storage_root: '/home/user/.gistclaw',
								approval_mode: 'prompt',
								approval_mode_label: 'Prompt',
								host_access_mode: 'standard',
								host_access_mode_label: 'Standard',
								admin_token: 'tok-123',
								per_run_token_budget: '50000',
								daily_cost_cap_usd: '5.00',
								rolling_cost_usd: 0.42,
								rolling_cost_label: '$0.42',
								telegram_token: '',
								active_project_name: 'my-project',
								active_project_path: '/home/user/my-project',
								active_project_summary: '3 agents'
							},
							access: {
								password_configured: true,
								other_active_devices: [],
								blocked_devices: []
							}
						}),
						{ status: 200, headers: { 'content-type': 'application/json' } }
					);
				case '/api/work':
					return new Response(
						JSON.stringify({
							active_project_name: 'my-project',
							active_project_path: '/home/user/my-project',
							queue_strip: {
								headline: '1 active run',
								root_runs: 1,
								worker_runs: 0,
								recovery_runs: 0,
								summary: {
									total: 1,
									pending: 0,
									active: 1,
									needs_approval: 0,
									completed: 0,
									failed: 0,
									interrupted: 0,
									root_status: 'active'
								}
							},
							paging: { has_next: false, has_prev: false },
							clusters: []
						}),
						{ status: 200, headers: { 'content-type': 'application/json' } }
					);
				case '/api/deliveries/health':
					return new Response(
						JSON.stringify({
							Connectors: [
								{
									ConnectorID: 'telegram',
									PendingCount: 2,
									RetryingCount: 1,
									TerminalCount: 0
								}
							],
							RuntimeConnectors: [
								{
									ConnectorID: 'telegram',
									State: 'degraded',
									Summary: 'poll loop stale',
									CheckedAt: '2026-03-29T10:00:00Z',
									RestartSuggested: true
								}
							]
						}),
						{ status: 200, headers: { 'content-type': 'application/json' } }
					);
				default:
					throw new Error(`unexpected input ${String(input)}`);
			}
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected debug load to return data');
		}

		expect(fetcher).toHaveBeenCalledWith('/api/settings', expect.any(Object));
		expect(fetcher).toHaveBeenCalledWith('/api/work', expect.any(Object));
		expect(fetcher).toHaveBeenCalledWith('/api/deliveries/health', expect.any(Object));
		expect(result.debug.settings?.machine.approval_mode_label).toBe('Prompt');
		expect(result.debug.work?.queue_strip.headline).toBe('1 active run');
		expect(result.debug.health.connectors[0].connector_id).toBe('telegram');
	});

	it('returns partial fallbacks when requests fail', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			if (String(input) === '/api/work') {
				return new Response(
					JSON.stringify({
						active_project_name: 'my-project',
						active_project_path: '/home/user/my-project',
						queue_strip: {
							headline: 'idle',
							root_runs: 0,
							worker_runs: 0,
							recovery_runs: 0,
							summary: {
								total: 0,
								pending: 0,
								active: 0,
								needs_approval: 0,
								completed: 0,
								failed: 0,
								interrupted: 0,
								root_status: 'idle'
							}
						},
						paging: { has_next: false, has_prev: false },
						clusters: []
					}),
					{ status: 200, headers: { 'content-type': 'application/json' } }
				);
			}

			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected debug load to return fallback data');
		}

		expect(result).toEqual({
			debug: {
				settings: null,
				work: {
					active_project_name: 'my-project',
					active_project_path: '/home/user/my-project',
					queue_strip: {
						headline: 'idle',
						root_runs: 0,
						worker_runs: 0,
						recovery_runs: 0,
						summary: {
							total: 0,
							pending: 0,
							active: 0,
							needs_approval: 0,
							completed: 0,
							failed: 0,
							interrupted: 0,
							root_status: 'idle'
						}
					},
					paging: { has_next: false, has_prev: false },
					clusters: []
				},
				health: {
					connectors: [],
					runtime_connectors: []
				}
			}
		});
	});
});
