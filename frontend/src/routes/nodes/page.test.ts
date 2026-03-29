import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import NodesPage from './+page.svelte';

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: [{ id: 'nodes', label: 'Nodes', href: '/nodes' }],
	onboarding: null,
	currentPath: '/nodes',
	currentSearch: ''
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
		expect(body).toContain('Inventory');
		expect(body).toContain('Deferred');
		expect(body).toContain('Capability Registry');
		expect(body).toContain('Project');
		expect(body).toContain('my-project');
		expect(body).toContain('/home/user/my-project');
	});

	it('renders node inventory guidance by default', () => {
		const { body } = render(NodesPage, { props: { data: baseData } });
		expect(body).toContain('Node inventory is waiting on a dedicated backend.');
		expect(body).toContain('Run Graph');
		expect(body).toContain('Debug');
		expect(body).toContain('Recovery');
	});

	it('renders capabilities guidance when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=capabilities' };
		const { body } = render(NodesPage, { props: { data } });
		expect(body).toContain('Capability seams ship before node inventory.');
		expect(body).toContain('connector_inbox_list');
		expect(body).toContain('connector_send');
		expect(body).toContain('system.run');
	});
});
