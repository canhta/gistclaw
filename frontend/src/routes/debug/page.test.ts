import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import DebugPage from './+page.svelte';

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: [{ id: 'debug', label: 'Debug', href: '/debug' }],
	onboarding: null,
	currentPath: '/debug',
	currentSearch: '',
	debug: {
		settings: {
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
		},
		work: {
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
			clusters: [
				{
					root: {
						id: 'run-root',
						objective: 'Repair connector backlog',
						agent_id: 'front',
						status: 'active',
						status_label: 'Active',
						status_class: 'is-active',
						model_display: 'gpt-5.4',
						token_summary: '1.2K tokens',
						started_at_short: '10:00',
						started_at_exact: '2026-03-29 10:00',
						started_at_iso: '2026-03-29T10:00:00Z',
						last_activity_short: '10:05',
						last_activity_exact: '2026-03-29 10:05',
						last_activity_iso: '2026-03-29T10:05:00Z',
						depth: 0
					},
					children: [
						{
							id: 'run-worker',
							objective: 'Collect connector evidence',
							agent_id: 'worker-1',
							status: 'active',
							status_label: 'Active',
							status_class: 'is-active',
							model_display: 'gpt-5.4-mini',
							token_summary: '320 tokens',
							started_at_short: '10:01',
							started_at_exact: '2026-03-29 10:01',
							started_at_iso: '2026-03-29T10:01:00Z',
							last_activity_short: '10:04',
							last_activity_exact: '2026-03-29 10:04',
							last_activity_iso: '2026-03-29T10:04:00Z',
							depth: 1
						}
					],
					child_count: 1,
					child_count_label: '1 child run',
					blocker_label: '',
					has_children: true
				}
			]
		},
		health: {
			connectors: [
				{ connector_id: 'telegram', pending_count: 2, retrying_count: 1, terminal_count: 0 }
			],
			runtime_connectors: [
				{
					connector_id: 'telegram',
					state: 'degraded',
					summary: 'poll loop stale',
					checked_at: '2026-03-29T10:00:00Z',
					restart_suggested: true
				}
			]
		},
		rpc: {
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
				},
				{
					name: 'connector_health',
					label: 'Connector health',
					description: 'Inspect configured connector health snapshots.'
				},
				{
					name: 'active_project',
					label: 'Active project',
					description: 'Inspect the current project scope and workspace path.'
				},
				{
					name: 'schedule_status',
					label: 'Scheduler',
					description: 'Inspect schedule counters and the next scheduler wake time.'
				}
			],
			result: {
				probe: 'status',
				label: 'Status',
				summary: 'system status loaded',
				executed_at: '2026-03-29T10:06:00Z',
				executed_at_label: '2026-03-29 10:06:00 UTC',
				data: {
					active_runs: 1,
					pending_approvals: 0,
					storage_backup: 'healthy'
				}
			}
		},
		events: {
			summary: {
				source_count: 2,
				event_count: 2,
				selected_run_id: 'run-root',
				latest_event_label: 'Approval Requested',
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
				},
				{
					run_id: 'run-worker',
					objective: 'Collect connector evidence',
					agent_id: 'worker-1',
					status: 'active',
					status_label: 'Active',
					event_count: 1,
					latest_event_at_label: '2026-03-29 10:04:00 UTC',
					stream_url: '/api/work/run-worker/events'
				}
			],
			events: [
				{
					id: 'evt-approval',
					run_id: 'run-root',
					run_short_id: 'run-root',
					objective: 'Repair connector backlog',
					agent_id: 'front',
					kind: 'approval_requested',
					kind_label: 'Approval Requested',
					payload_preview: '{"tool_name":"system.run"}',
					occurred_at: '2026-03-29T10:06:00Z',
					occurred_at_label: '2026-03-29 10:06:00 UTC'
				},
				{
					id: 'evt-tool',
					run_id: 'run-root',
					run_short_id: 'run-root',
					objective: 'Repair connector backlog',
					agent_id: 'front',
					kind: 'run_started',
					kind_label: 'Run Started',
					payload_preview: 'No payload',
					occurred_at: '2026-03-29T10:05:00Z',
					occurred_at_label: '2026-03-29 10:05:00 UTC'
				}
			]
		}
	}
};

describe('Debug page', () => {
	it('renders the Debug heading', () => {
		const { body } = render(DebugPage, { props: { data: baseData } });
		expect(body).toContain('Debug');
	});

	it('renders Status, Health, Models, Events, RPC tabs', () => {
		const { body } = render(DebugPage, { props: { data: baseData } });
		expect(body).toContain('Status');
		expect(body).toContain('Health');
		expect(body).toContain('Models');
		expect(body).toContain('Events');
		expect(body).toContain('RPC');
	});

	it('renders status data by default', () => {
		const { body } = render(DebugPage, { props: { data: baseData } });
		expect(body).toContain('1 active run');
		expect(body).toContain('Prompt');
		expect(body).toContain('Standard');
		expect(body).toContain('$0.42');
	});

	it('renders health tab details when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=health' };
		const { body } = render(DebugPage, { props: { data } });
		expect(body).toContain('telegram');
		expect(body).toContain('poll loop stale');
		expect(body).toContain('2 pending');
	});

	it('renders model usage when the models tab is selected', () => {
		const data = { ...baseData, currentSearch: 'tab=models' };
		const { body } = render(DebugPage, { props: { data } });
		expect(body).toContain('Model usage');
		expect(body).toContain('gpt-5.4');
		expect(body).toContain('gpt-5.4-mini');
		expect(body).toContain('2 runs');
	});

	it('renders the recent event board when the events tab is selected', () => {
		const data = { ...baseData, currentSearch: 'tab=events' };
		const { body } = render(DebugPage, { props: { data } });
		expect(body).toContain('Recent event log');
		expect(body).toContain('Approval Requested');
		expect(body).toContain('Repair connector backlog');
		expect(body).toContain('Latest Event');
		expect(body).toContain('/api/work/run-root/events');
		expect(body).toContain('/api/work/run-worker/events');
		expect(body).toContain('Run Started');
	});

	it('renders the selected event source when the events tab chooses a run', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=events&run_id=run-worker',
			debug: {
				...baseData.debug,
				events: {
					...baseData.debug.events,
					summary: {
						...baseData.debug.events.summary,
						selected_run_id: 'run-worker',
						event_count: 1,
						latest_event_label: 'Tool Started'
					},
					filters: {
						...baseData.debug.events.filters,
						run_id: 'run-worker'
					},
					events: [
						{
							id: 'evt-worker',
							run_id: 'run-worker',
							run_short_id: 'run-worker',
							objective: 'Collect connector evidence',
							agent_id: 'worker-1',
							kind: 'tool_started',
							kind_label: 'Tool Started',
							payload_preview: '{"tool_name":"connector_send"}',
							occurred_at: '2026-03-29T10:04:00Z',
							occurred_at_label: '2026-03-29 10:04:00 UTC'
						}
					]
				}
			}
		};
		const { body } = render(DebugPage, { props: { data } });
		expect(body).toContain('Collect connector evidence');
		expect(body).toContain('worker-1');
		expect(body).toContain('Tool Started');
	});

	it('renders the RPC probe console when rpc tab is selected', () => {
		const data = { ...baseData, currentSearch: 'tab=rpc' };
		const { body } = render(DebugPage, { props: { data } });
		expect(body).toContain('RPC probes');
		expect(body).toContain('Read-only app probes');
		expect(body).toContain('Connector health');
		expect(body).toContain('Run probe');
		expect(body).toContain('system status loaded');
		expect(body).toContain('active_runs');
	});

	it('renders the selected rpc probe result when a non-default probe is loaded', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=rpc&probe=connector_health',
			debug: {
				...baseData.debug,
				rpc: {
					...baseData.debug.rpc,
					summary: {
						...baseData.debug.rpc.summary,
						selected_probe: 'connector_health'
					},
					result: {
						probe: 'connector_health',
						label: 'Connector health',
						summary: '1 connector snapshot loaded',
						executed_at: '2026-03-29T10:06:00Z',
						executed_at_label: '2026-03-29 10:06:00 UTC',
						data: {
							summary: {
								total: 1,
								healthy: 0,
								degraded: 1
							}
						}
					}
				}
			}
		};
		const { body } = render(DebugPage, { props: { data } });
		expect(body).toContain('Connector health');
		expect(body).toContain('1 connector snapshot loaded');
		expect(body).toContain('degraded');
	});

	it('renders the rpc board with a fallback notice instead of hiding it', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=rpc',
			debug: {
				...baseData.debug,
				rpc: {
					notice: 'RPC probes could not be loaded. Reload to retry.',
					summary: {
						probe_count: 4,
						read_only: true,
						default_probe: 'status',
						selected_probe: 'status'
					},
					probes: baseData.debug.rpc.probes,
					result: {
						probe: 'status',
						label: 'Status',
						summary: 'RPC probes could not be loaded. Reload to retry.',
						executed_at: '',
						executed_at_label: 'Unavailable',
						data: {}
					}
				}
			}
		};
		const { body } = render(DebugPage, { props: { data } });
		expect(body).toContain('RPC probes');
		expect(body).toContain('RPC probes could not be loaded. Reload to retry.');
		expect(body).toContain('Probe catalog');
		expect(body).toContain('Result');
		expect(body).not.toContain('RPC probes are currently unavailable from this daemon.');
	});
});
