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
			clusters: []
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

	it('renders RPC warning state when rpc tab is selected', () => {
		const data = { ...baseData, currentSearch: 'tab=rpc' };
		const { body } = render(DebugPage, { props: { data } });
		expect(body).toContain('RPC console');
		expect(body).toContain('High risk');
		expect(body).toContain('Unlock RPC Console');
	});
});
