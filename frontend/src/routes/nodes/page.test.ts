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

	it('renders COMING SOON state', () => {
		const { body } = render(NodesPage, { props: { data: baseData } });
		expect(body).toContain('COMING SOON');
	});
});
