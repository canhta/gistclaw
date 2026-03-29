import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ConfigPage from './+page.svelte';

const nav = [{ id: 'config', label: 'Config', href: '/config' }];

const machineSettings = {
	storage_root: '/home/user/.gistclaw',
	approval_mode: 'on_request',
	approval_mode_label: 'On Request',
	host_access_mode: 'local',
	host_access_mode_label: 'Local',
	admin_token: 'tok-123',
	per_run_token_budget: '50000',
	daily_cost_cap_usd: '5.00',
	rolling_cost_usd: 0.42,
	rolling_cost_label: '$0.42',
	telegram_token: '',
	active_project_name: 'my-project',
	active_project_path: '/home/user/my-project',
	active_project_summary: '3 agents'
};

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: nav,
	onboarding: null,
	currentPath: '/config',
	currentSearch: '',
	config: {
		settings: {
			machine: machineSettings,
			access: {
				password_configured: true,
				other_active_devices: [],
				blocked_devices: []
			}
		}
	}
};

describe('Config page', () => {
	it('renders the Config heading', () => {
		const { body } = render(ConfigPage, { props: { data: baseData } });
		expect(body).toContain('Config');
	});

	it('renders General, Agents & Routing, Models, Channels, Raw JSON5, Apply tabs', () => {
		const { body } = render(ConfigPage, { props: { data: baseData } });
		expect(body).toContain('General');
		expect(body).toContain('Agents &amp; Routing');
		expect(body).toContain('Models');
		expect(body).toContain('Channels');
		expect(body).toContain('Raw JSON5');
		expect(body).toContain('Apply');
	});

	it('renders token budget value from settings', () => {
		const { body } = render(ConfigPage, { props: { data: baseData } });
		expect(body).toContain('50000');
	});

	it('renders rolling cost label', () => {
		const { body } = render(ConfigPage, { props: { data: baseData } });
		expect(body).toContain('$0.42');
	});

	it('renders error state when settings is null', () => {
		const data = { ...baseData, config: { settings: null } };
		const { body } = render(ConfigPage, { props: { data } });
		expect(body).toContain('Failed to load');
	});
});
