import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(
	fetcher: typeof fetch,
	url = 'http://localhost:3000/debug'
): Parameters<typeof load>[0] {
	return {
		fetch: fetcher,
		url: new URL(url)
	} as unknown as Parameters<typeof load>[0];
}

describe('debug load', () => {
	it('loads settings, work queue, delivery health, rpc probes, and debug events in parallel', async () => {
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
				case '/api/debug/rpc':
					return new Response(
						JSON.stringify({
							summary: {
								probe_count: 4,
								read_only: true,
								default_probe: 'status',
								selected_probe: 'status'
							},
							probes: [
								{
									name: 'status',
									label: 'Status',
									description: 'Inspect active runs, approvals, and storage health.'
								}
							],
							result: {
								probe: 'status',
								label: 'Status',
								summary: 'system status loaded',
								executed_at: '2026-03-29T10:06:00Z',
								executed_at_label: '2026-03-29 10:06:00 UTC',
								data: {
									active_runs: 1
								}
							}
						}),
						{ status: 200, headers: { 'content-type': 'application/json' } }
					);
				case '/api/debug/events':
					return new Response(
						JSON.stringify({
							summary: {
								source_count: 1,
								event_count: 2,
								selected_run_id: 'run-root',
								latest_event_label: 'Run Started',
								latest_event_at_label: '2026-03-29 10:06:00 UTC'
							},
							filters: {
								run_id: 'run-root',
								limit: 20
							},
							sources: [
								{
									run_id: 'run-root',
									objective: 'Repair connector backlog',
									agent_id: 'front',
									status: 'active',
									status_label: 'Active',
									event_count: 2,
									latest_event_at_label: '2026-03-29 10:06:00 UTC',
									stream_url: '/api/work/run-root/events'
								}
							],
							events: [
								{
									id: 'evt-2',
									run_id: 'run-root',
									run_short_id: 'run-root',
									objective: 'Repair connector backlog',
									agent_id: 'front',
									kind: 'run_started',
									kind_label: 'Run Started',
									payload_preview: 'No payload',
									occurred_at: '2026-03-29T10:06:00Z',
									occurred_at_label: '2026-03-29 10:06:00 UTC'
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
		expect(fetcher).toHaveBeenCalledWith('/api/debug/rpc', expect.any(Object));
		expect(fetcher).toHaveBeenCalledWith('/api/debug/events', expect.any(Object));
		expect(result.debug.settings?.machine.approval_mode_label).toBe('Prompt');
		expect(result.debug.work?.queue_strip.headline).toBe('1 active run');
		expect(result.debug.health.connectors[0].connector_id).toBe('telegram');
		expect(result.debug.rpc?.summary.selected_probe).toBe('status');
		expect(result.debug.events?.summary.selected_run_id).toBe('run-root');
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
				},
				rpc: {
					notice: 'RPC probes could not be loaded. Reload to retry.',
					summary: {
						probe_count: 4,
						read_only: true,
						default_probe: 'status',
						selected_probe: 'status'
					},
					probes: [
						expect.objectContaining({
							name: 'status',
							label: 'Status'
						}),
						expect.objectContaining({
							name: 'connector_health',
							label: 'Connector health'
						}),
						expect.objectContaining({
							name: 'active_project',
							label: 'Active project'
						}),
						expect.objectContaining({
							name: 'schedule_status',
							label: 'Scheduler'
						})
					],
					result: {
						probe: 'status',
						label: 'Status',
						summary: 'RPC probes could not be loaded. Reload to retry.',
						executed_at: '',
						executed_at_label: 'Unavailable',
						data: {}
					}
				},
				events: null
			}
		});
	});

	it('loads the requested rpc probe when the search param selects one', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			switch (String(input)) {
				case '/api/settings':
				case '/api/work':
				case '/api/deliveries/health':
					return new Response('{}', {
						status: 200,
						headers: { 'content-type': 'application/json' }
					});
				case '/api/debug/rpc?probe=connector_health':
					return new Response(
						JSON.stringify({
							summary: {
								probe_count: 4,
								read_only: true,
								default_probe: 'status',
								selected_probe: 'connector_health'
							},
							probes: [],
							result: {
								probe: 'connector_health',
								label: 'Connector health',
								summary: 'ready',
								executed_at: '2026-03-29T10:06:00Z',
								executed_at_label: '2026-03-29 10:06:00 UTC',
								data: {}
							}
						}),
						{ status: 200, headers: { 'content-type': 'application/json' } }
					);
				default:
					throw new Error(`unexpected input ${String(input)}`);
			}
		});

		await load(
			makeLoadEvent(fetcher, 'http://localhost:3000/debug?tab=rpc&probe=connector_health')
		);

		expect(fetcher).toHaveBeenCalledWith(
			'/api/debug/rpc?probe=connector_health',
			expect.any(Object)
		);
	});

	it('loads the requested debug events run when the search param selects one', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			switch (String(input)) {
				case '/api/settings':
				case '/api/work':
				case '/api/deliveries/health':
				case '/api/debug/rpc':
					return new Response('{}', {
						status: 200,
						headers: { 'content-type': 'application/json' }
					});
				case '/api/debug/events?run_id=run-worker':
					return new Response(
						JSON.stringify({
							summary: {
								source_count: 1,
								event_count: 1,
								selected_run_id: 'run-worker',
								latest_event_label: 'Tool Started',
								latest_event_at_label: '2026-03-29 10:07:00 UTC'
							},
							filters: { run_id: 'run-worker', limit: 20 },
							sources: [],
							events: []
						}),
						{ status: 200, headers: { 'content-type': 'application/json' } }
					);
				default:
					throw new Error(`unexpected input ${String(input)}`);
			}
		});

		await load(makeLoadEvent(fetcher, 'http://localhost:3000/debug?tab=events&run_id=run-worker'));

		expect(fetcher).toHaveBeenCalledWith('/api/debug/events?run_id=run-worker', expect.any(Object));
	});
});
