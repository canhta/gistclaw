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
		team: {
			notice: 'Loaded from team file',
			active_profile: {
				id: 'default',
				label: 'default',
				active: true,
				save_path: '/home/user/.gistclaw/profiles/default.json5'
			},
			profiles: [
				{
					id: 'default',
					label: 'default',
					active: true,
					save_path: '/home/user/.gistclaw/profiles/default.json5'
				},
				{
					id: 'safe',
					label: 'safe',
					active: false,
					save_path: '/home/user/.gistclaw/profiles/safe.json5'
				}
			],
			team: {
				name: 'Repo Task Team',
				front_agent_id: 'assistant',
				member_count: 3,
				members: [
					{
						id: 'assistant',
						role: 'front assistant',
						soul_file: 'teams/assistant.md',
						base_profile: 'default',
						tool_families: ['repo_read', 'web_fetch'],
						delegation_kinds: ['reviewer', 'patcher'],
						can_message: ['reviewer', 'patcher'],
						specialist_summary_visibility: 'full',
						soul_extra: {},
						is_front: true
					},
					{
						id: 'reviewer',
						role: 'diff reviewer',
						soul_file: 'teams/reviewer.md',
						base_profile: 'default',
						tool_families: ['repo_read'],
						delegation_kinds: [],
						can_message: [],
						specialist_summary_visibility: 'summary',
						soul_extra: {},
						is_front: false
					},
					{
						id: 'patcher',
						role: 'repo patcher',
						soul_file: 'teams/patcher.md',
						base_profile: 'safe',
						tool_families: ['repo_read', 'repo_write'],
						delegation_kinds: [],
						can_message: [],
						specialist_summary_visibility: 'summary',
						soul_extra: {},
						is_front: false
					}
				]
			}
		},
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

	it('renders team-backed agents and routing details when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=agents' };
		const { body } = render(ConfigPage, { props: { data } });
		expect(body).toContain('Repo Task Team');
		expect(body).toContain('Front Agent');
		expect(body).toContain('assistant');
		expect(body).toContain('front assistant');
		expect(body).toContain('diff reviewer');
		expect(body).toContain('repo patcher');
		expect(body).toContain('reviewer');
		expect(body).toContain('patcher');
		expect(body).toContain('repo_read');
		expect(body).toContain('default');
		expect(body).toContain('/home/user/.gistclaw/profiles/default.json5');
	});

	it('renders a team-unavailable message when /api/team data is missing', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=agents',
			config: {
				...baseData.config,
				team: null
			}
		};
		const { body } = render(ConfigPage, { props: { data } });
		expect(body).toContain('Team surface unavailable');
		expect(body).toContain('/api/team');
	});

	it('renders error state when settings is null', () => {
		const data = { ...baseData, config: { ...baseData.config, settings: null } };
		const { body } = render(ConfigPage, { props: { data } });
		expect(body).toContain('Failed to load');
	});
});
