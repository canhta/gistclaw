import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import LogsPage from './+page.svelte';

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: [{ id: 'logs', label: 'Logs', href: '/logs' }],
	onboarding: null,
	currentPath: '/logs',
	currentSearch: ''
};

describe('Logs page', () => {
	it('renders the Logs heading', () => {
		const { body } = render(LogsPage, { props: { data: baseData } });
		expect(body).toContain('Logs');
	});

	it('renders Live Tail, Filters, Export tabs', () => {
		const { body } = render(LogsPage, { props: { data: baseData } });
		expect(body).toContain('Live Tail');
		expect(body).toContain('Filters');
		expect(body).toContain('Export');
	});

	it('renders COMING SOON state', () => {
		const { body } = render(LogsPage, { props: { data: baseData } });
		expect(body).toContain('COMING SOON');
	});
});
