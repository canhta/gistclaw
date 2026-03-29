import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import UpdatePage from './+page.svelte';

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: [{ id: 'update', label: 'Update', href: '/update' }],
	onboarding: null,
	currentPath: '/update',
	currentSearch: ''
};

describe('Update page', () => {
	it('renders the Update heading', () => {
		const { body } = render(UpdatePage, { props: { data: baseData } });
		expect(body).toContain('Update');
	});

	it('renders Run Update and Restart Report tabs', () => {
		const { body } = render(UpdatePage, { props: { data: baseData } });
		expect(body).toContain('Run Update');
		expect(body).toContain('Restart Report');
	});

	it('renders COMING SOON state', () => {
		const { body } = render(UpdatePage, { props: { data: baseData } });
		expect(body).toContain('COMING SOON');
	});
});
