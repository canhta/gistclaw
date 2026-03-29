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

	it('renders presence summary cards and project context', () => {
		const { body } = render(InstancesPage, { props: { data: baseData } });
		expect(body).toContain('Presence Feed');
		expect(body).toContain('Deferred');
		expect(body).toContain('Worker Sessions');
		expect(body).toContain('Project');
		expect(body).toContain('my-project');
		expect(body).toContain('/home/user/my-project');
	});

	it('renders runtime presence guidance by default', () => {
		const { body } = render(InstancesPage, { props: { data: baseData } });
		expect(body).toContain('Instance presence is waiting on a dedicated backend.');
		expect(body).toContain('runtime-managed typing');
		expect(body).toContain('Chat');
		expect(body).toContain('Channels');
	});

	it('renders details guidance when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=details' };
		const { body } = render(InstancesPage, { props: { data } });
		expect(body).toContain('Instance detail remains derived from live surfaces.');
		expect(body).toContain('Sessions');
		expect(body).toContain('Debug');
		expect(body).toContain('Recover');
	});
});
