import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import InstancesPage from './+page.svelte';

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: [{ id: 'instances', label: 'Instances', href: '/instances' }],
	onboarding: null,
	currentPath: '/instances',
	currentSearch: ''
};

describe('Instances page', () => {
	it('renders the Instances heading', () => {
		const { body } = render(InstancesPage, { props: { data: baseData } });
		expect(body).toContain('Instances');
	});

	it('renders Presence and Details tabs', () => {
		const { body } = render(InstancesPage, { props: { data: baseData } });
		expect(body).toContain('Presence');
		expect(body).toContain('Details');
	});

	it('renders COMING SOON state', () => {
		const { body } = render(InstancesPage, { props: { data: baseData } });
		expect(body).toContain('COMING SOON');
	});
});
