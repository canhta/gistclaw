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

const workData = {
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
			needs_approval: 0,
			completed: 1,
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
			children: [
				{
					id: 'run-child',
					objective: 'Check tests',
					agent_id: 'reviewer',
					status: 'completed',
					status_label: 'Completed',
					status_class: 'is-success',
					model_display: 'gpt-5.4-mini',
					token_summary: '400 tokens',
					started_at_short: '10:01',
					started_at_exact: '2026-03-29 10:01',
					started_at_iso: '2026-03-29T10:01:00Z',
					last_activity_short: '10:03',
					last_activity_exact: '2026-03-29 10:03',
					last_activity_iso: '2026-03-29T10:03:00Z',
					depth: 1
				}
			],
			child_count: 1,
			child_count_label: '1 child run',
			blocker_label: '',
			has_children: true
		}
	]
};

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: nav,
	onboarding: null,
	currentPath: '/config',
	currentSearch: '',
	config: {
		work: workData,
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
				current_device: {
					id: 'device-current',
					primary_label: 'MacBook Pro',
					secondary_line: 'Current browser',
					current: true,
					blocked: false,
					active_sessions: 1,
					details_ip: '127.0.0.1',
					details_user_agent: 'Safari 17'
				},
				other_active_devices: [
					{
						id: 'device-other',
						primary_label: 'Windows Chrome',
						secondary_line: 'Signed in 5 minutes ago',
						current: false,
						blocked: false,
						active_sessions: 2,
						details_ip: '10.0.0.5',
						details_user_agent: 'Chrome 123'
					}
				],
				blocked_devices: [
					{
						id: 'device-blocked',
						primary_label: 'Linux Firefox',
						secondary_line: 'Blocked yesterday',
						current: false,
						blocked: true,
						active_sessions: 0,
						details_ip: '10.0.0.8',
						details_user_agent: 'Firefox 124'
					}
				]
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

	it('renders browser access and password state on the General tab', () => {
		const { body } = render(ConfigPage, { props: { data: baseData } });
		expect(body).toContain('Browser access');
		expect(body).toContain('Password set');
		expect(body).toContain('MacBook Pro');
		expect(body).toContain('Windows Chrome');
		expect(body).toContain('Linux Firefox');
		expect(body).toContain('Revoke');
		expect(body).toContain('Block');
		expect(body).toContain('Unblock');
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
		expect(body).toContain('Switch profile');
		expect(body).toContain('Create profile');
		expect(body).toContain('Clone profile');
		expect(body).toContain('Delete profile');
		expect(body).toContain('/api/team/export');
		expect(body).toContain('Export team file');
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

	it('renders model posture and recent usage when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=models' };
		const { body } = render(ConfigPage, { props: { data } });
		expect(body).toContain('Recent model usage');
		expect(body).toContain('gpt-5.4');
		expect(body).toContain('gpt-5.4-mini');
		expect(body).toContain('Anthropic + OpenAI-compatible');
		expect(body).toContain('Runtime-owned');
	});

	it('renders a no-evidence state when recent work data is missing', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=models',
			config: {
				...baseData.config,
				work: null
			}
		};
		const { body } = render(ConfigPage, { props: { data } });
		expect(body).toContain('No recent model evidence');
	});

	it('renders error state when settings is null', () => {
		const data = { ...baseData, config: { ...baseData.config, settings: null } };
		const { body } = render(ConfigPage, { props: { data } });
		expect(body).toContain('Failed to load');
	});
});
