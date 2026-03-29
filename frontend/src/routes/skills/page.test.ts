import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SkillsPage from './+page.svelte';

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: [{ id: 'skills', label: 'Skills', href: '/skills' }],
	onboarding: null,
	currentPath: '/skills',
	currentSearch: ''
};

describe('Skills page', () => {
	it('renders the Skills heading', () => {
		const { body } = render(SkillsPage, { props: { data: baseData } });
		expect(body).toContain('Skills');
	});

	it('renders Installed, Available, Credentials tabs', () => {
		const { body } = render(SkillsPage, { props: { data: baseData } });
		expect(body).toContain('Installed');
		expect(body).toContain('Available');
		expect(body).toContain('Credentials');
	});

	it('renders COMING SOON state', () => {
		const { body } = render(SkillsPage, { props: { data: baseData } });
		expect(body).toContain('COMING SOON');
	});
});
