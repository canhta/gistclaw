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

	it('renders extension summary cards and project context', () => {
		const { body } = render(SkillsPage, { props: { data: baseData } });
		expect(body).toContain('Extension Seams');
		expect(body).toContain('3');
		expect(body).toContain('Workflow');
		expect(body).toContain('Project');
		expect(body).toContain('my-project');
		expect(body).toContain('/home/user/my-project');
	});

	it('renders installed-skill guidance by default', () => {
		const { body } = render(SkillsPage, { props: { data: baseData } });
		expect(body).toContain('Installed skills are managed in the repo today.');
		expect(body).toContain('Tools');
		expect(body).toContain('Providers');
		expect(body).toContain('Connectors');
	});

	it('renders availability guidance when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=available' };
		const { body } = render(SkillsPage, { props: { data } });
		expect(body).toContain('Available skills');
		expect(body).toContain('No marketplace or install workflow is wired yet.');
		expect(body).toContain('Workspace-owned packs');
	});

	it('renders credentials guidance when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=credentials' };
		const { body } = render(SkillsPage, { props: { data } });
		expect(body).toContain('Keep secrets explicit');
		expect(body).toContain('Provider keys and connector secrets still live in config.');
		expect(body).toContain('Credentials UI is deferred');
	});
});
