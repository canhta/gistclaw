import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import DebugPage from './+page.svelte';

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: [{ id: 'debug', label: 'Debug', href: '/debug' }],
	onboarding: null,
	currentPath: '/debug',
	currentSearch: ''
};

describe('Debug page', () => {
	it('renders the Debug heading', () => {
		const { body } = render(DebugPage, { props: { data: baseData } });
		expect(body).toContain('Debug');
	});

	it('renders Status, Health, Models, Events, RPC tabs', () => {
		const { body } = render(DebugPage, { props: { data: baseData } });
		expect(body).toContain('Status');
		expect(body).toContain('Health');
		expect(body).toContain('Models');
		expect(body).toContain('Events');
		expect(body).toContain('RPC');
	});

	it('renders COMING SOON state', () => {
		const { body } = render(DebugPage, { props: { data: baseData } });
		expect(body).toContain('COMING SOON');
	});
});
