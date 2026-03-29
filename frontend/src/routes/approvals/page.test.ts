import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ApprovalsPage from './+page.svelte';

const nav = [{ id: 'approvals', label: 'Exec Approvals', href: '/approvals' }];

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: nav,
	onboarding: null,
	currentPath: '/approvals',
	currentSearch: '',
	approvals: {
		items: [],
		paging: { has_next: false, has_prev: false },
		openCount: 0,
		summary: {
			pendingCount: 0,
			connectorCount: 2,
			activeRoutes: 0
		},
		policy: {
			summary: {
				node_count: 2,
				allowlist_count: 3,
				pending_agents: 1,
				override_agents: 1
			},
			gateway: {
				approval_mode: 'prompt',
				approval_mode_label: 'Prompt',
				host_access_mode: 'standard',
				host_access_mode_label: 'Standard',
				team_name: 'Repo Task Team',
				front_agent_id: 'assistant'
			},
			nodes: [
				{
					agent_id: 'assistant',
					role: 'front assistant',
					base_profile: 'operator',
					is_front: true,
					tool_families: ['repo_read', 'delegate'],
					delegation_kinds: ['write', 'review'],
					can_message: ['patcher'],
					allow_tools: ['shell_exec', 'connector_send'],
					deny_tools: ['app_action'],
					pending_approvals: 0,
					recent_runs: 2,
					override_runs: 0,
					observed_approval_mode: 'prompt',
					observed_approval_mode_label: 'Prompt',
					observed_host_access_mode: 'standard',
					observed_host_access_mode_label: 'Standard'
				},
				{
					agent_id: 'patcher',
					role: 'scoped write specialist',
					base_profile: 'write',
					is_front: false,
					tool_families: ['repo_read', 'repo_write'],
					delegation_kinds: [],
					can_message: ['assistant'],
					allow_tools: [],
					deny_tools: ['repo_exec'],
					pending_approvals: 1,
					recent_runs: 1,
					override_runs: 1,
					observed_approval_mode: 'auto_approve',
					observed_approval_mode_label: 'Auto approve',
					observed_host_access_mode: 'elevated',
					observed_host_access_mode_label: 'Elevated'
				}
			],
			allowlists: [
				{
					agent_id: 'assistant',
					role: 'front assistant',
					tool_name: 'connector_send',
					direction: 'allow',
					direction_label: 'Allow'
				},
				{
					agent_id: 'assistant',
					role: 'front assistant',
					tool_name: 'shell_exec',
					direction: 'allow',
					direction_label: 'Allow'
				},
				{
					agent_id: 'assistant',
					role: 'front assistant',
					tool_name: 'app_action',
					direction: 'deny',
					direction_label: 'Deny'
				},
				{
					agent_id: 'patcher',
					role: 'scoped write specialist',
					tool_name: 'repo_exec',
					direction: 'deny',
					direction_label: 'Deny'
				}
			]
		}
	}
};

describe('Exec Approvals page', () => {
	it('renders the Exec Approvals heading', () => {
		const { body } = render(ApprovalsPage, { props: { data: baseData } });
		expect(body).toContain('Exec Approvals');
	});

	it('renders Gateway, Nodes, Allowlists tabs', () => {
		const { body } = render(ApprovalsPage, { props: { data: baseData } });
		expect(body).toContain('Gateway');
		expect(body).toContain('Nodes');
		expect(body).toContain('Allowlists');
	});

	it('renders approval summary cards and project context', () => {
		const data = {
			...baseData,
			approvals: {
				...baseData.approvals,
				openCount: 1,
				summary: {
					pendingCount: 1,
					connectorCount: 2,
					activeRoutes: 3
				}
			}
		};
		const { body } = render(ApprovalsPage, { props: { data } });
		expect(body).toContain('Open Queue');
		expect(body).toContain('Connected Lanes');
		expect(body).toContain('Project');
		expect(body).toContain('my-project');
		expect(body).toContain('/home/user/my-project');
	});

	it('renders empty state when no pending approvals', () => {
		const { body } = render(ApprovalsPage, { props: { data: baseData } });
		expect(body).toContain('No pending approvals');
		expect(body).toContain('The gateway is clear');
	});

	it('renders approval row when approvals are provided', () => {
		const data = {
			...baseData,
			approvals: {
				...baseData.approvals,
				items: [
					{
						id: 'appr-1',
						run_id: 'run-abc',
						tool_name: 'bash',
						binding_summary: 'echo hello',
						status: 'pending',
						status_label: 'Pending',
						status_class: 'is-active'
					}
				],
				paging: { has_next: false, has_prev: false },
				openCount: 1,
				summary: {
					pendingCount: 1,
					connectorCount: 2,
					activeRoutes: 1
				}
			}
		};
		const { body } = render(ApprovalsPage, { props: { data } });
		expect(body).toContain('bash');
		expect(body).toContain('echo hello');
		expect(body).toContain('APPROVE');
		expect(body).toContain('DENY');
	});

	it('only renders pending approvals in the gateway queue', () => {
		const data = {
			...baseData,
			approvals: {
				...baseData.approvals,
				items: [
					{
						id: 'appr-1',
						run_id: 'run-abc',
						tool_name: 'bash',
						binding_summary: 'echo hello',
						status: 'pending',
						status_label: 'Pending',
						status_class: 'is-active'
					},
					{
						id: 'appr-2',
						run_id: 'run-def',
						tool_name: 'read_file',
						binding_summary: '/etc/hosts',
						status: 'approved',
						status_label: 'Approved',
						status_class: 'is-success'
					}
				],
				paging: { has_next: false, has_prev: false },
				openCount: 1,
				summary: {
					pendingCount: 1,
					connectorCount: 2,
					activeRoutes: 1
				}
			}
		};
		const { body } = render(ApprovalsPage, { props: { data } });
		expect(body).toContain('bash');
		expect(body).not.toContain('read_file');
	});

	it('renders node policy board when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=nodes' };
		const { body } = render(ApprovalsPage, { props: { data } });
		expect(body).toContain('Observed node approval posture');
		expect(body).toContain('assistant');
		expect(body).toContain('patcher');
		expect(body).toContain('Auto approve');
		expect(body).toContain('repo_write');
		expect(body).toContain('repo_exec');
	});

	it('renders allowlist board when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=allowlists' };
		const { body } = render(ApprovalsPage, { props: { data } });
		expect(body).toContain('Explicit tool allowlists');
		expect(body).toContain('connector_send');
		expect(body).toContain('shell_exec');
		expect(body).toContain('app_action');
		expect(body).toContain('repo_exec');
	});
});
