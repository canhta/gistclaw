import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import NodesPage from './+page.svelte';

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: [{ id: 'nodes', label: 'Nodes', href: '/nodes' }],
	onboarding: null,
	currentPath: '/nodes',
	currentSearch: '',
	nodesLoadError: '',
	nodes: {
		summary: {
			connectors: 2,
			healthy_connectors: 1,
			run_nodes: 2,
			approval_nodes: 1,
			capabilities: 3
		},
		connectors: [
			{
				id: 'telegram',
				aliases: ['tg'],
				exposure: 'remote',
				state: 'healthy',
				state_label: 'healthy',
				summary: 'polling',
				checked_at_label: '2026-03-29 09:30:00 UTC',
				restart_suggested: false
			},
			{
				id: 'whatsapp',
				aliases: [],
				exposure: 'remote',
				state: 'degraded',
				state_label: 'degraded',
				summary: 'token expired',
				checked_at_label: '2026-03-29 09:31:00 UTC',
				restart_suggested: true
			}
		],
		runs: [
			{
				id: 'run-root',
				short_id: 'run-root',
				parent_run_id: '',
				kind: 'root',
				agent_id: 'assistant',
				status: 'active',
				status_label: 'active',
				objective_preview: 'Review the repo layout',
				started_at_label: '2026-03-29 09:30:00 UTC',
				updated_at_label: '2026-03-29 09:31:00 UTC'
			},
			{
				id: 'run-worker',
				short_id: 'run-worker',
				parent_run_id: 'run-root',
				kind: 'worker',
				agent_id: 'researcher',
				status: 'needs_approval',
				status_label: 'needs approval',
				objective_preview: 'Inspect the docs',
				started_at_label: '2026-03-29 09:32:00 UTC',
				updated_at_label: '2026-03-29 09:33:00 UTC'
			}
		],
		capabilities: [
			{
				name: 'connector_inbox_list',
				family: 'connector',
				description: 'List connector inbox threads.'
			},
			{
				name: 'connector_send',
				family: 'connector',
				description: 'Send a direct message through a configured connector.'
			},
			{
				name: 'app_action',
				family: 'app',
				description: 'Execute a runtime app action.'
			}
		]
	}
};

describe('Nodes page', () => {
	it('renders the Nodes heading', () => {
		const { body } = render(NodesPage, { props: { data: baseData } });
		expect(body).toContain('Nodes');
	});

	it('renders List and Capabilities tabs', () => {
		const { body } = render(NodesPage, { props: { data: baseData } });
		expect(body).toContain('List');
		expect(body).toContain('Capabilities');
	});

	it('renders node summary cards and project context', () => {
		const { body } = render(NodesPage, { props: { data: baseData } });
		expect(body).toContain('Configured Connectors');
		expect(body).toContain('2');
		expect(body).toContain('Healthy Connectors');
		expect(body).toContain('Project');
		expect(body).toContain('my-project');
		expect(body).toContain('/home/user/my-project');
	});

	it('renders the node inventory board by default', () => {
		const { body } = render(NodesPage, { props: { data: baseData } });
		expect(body).toContain('Configured connector inventory');
		expect(body).toContain('telegram');
		expect(body).toContain('whatsapp');
		expect(body).toContain('run-worker');
		expect(body).toContain('needs approval');
	});

	it('renders the capabilities board when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=capabilities' };
		const { body } = render(NodesPage, { props: { data } });
		expect(body).toContain('Direct runtime capability tools');
		expect(body).toContain('connector_inbox_list');
		expect(body).toContain('connector_send');
		expect(body).toContain('app_action');
	});

	it('renders a load error panel when node inventory is unavailable', () => {
		const data = {
			...baseData,
			nodes: null,
			nodesLoadError: 'Node inventory could not be loaded. Reload to retry.'
		};
		const { body } = render(NodesPage, { props: { data } });
		expect(body).toContain('Node inventory could not be loaded. Reload to retry.');
		expect(body).toContain('Nodes board unavailable');
		expect(body).not.toContain('Configured connector inventory');
	});
});
