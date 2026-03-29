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

	it('renders node policy guidance when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=nodes' };
		const { body } = render(ApprovalsPage, { props: { data } });
		expect(body).toContain('Node approval policy remains centralized at the gateway.');
		expect(body).toContain('worker sessions');
		expect(body).toContain('Debug');
		expect(body).toContain('Sessions');
	});

	it('renders allowlist guidance when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=allowlists' };
		const { body } = render(ApprovalsPage, { props: { data } });
		expect(body).toContain('Allowlists are still managed outside the browser.');
		expect(body).toContain('Config');
		expect(body).toContain('Gateway queue');
		expect(body).toContain('Chat');
	});
});
