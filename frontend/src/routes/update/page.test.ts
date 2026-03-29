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

	it('renders maintenance summary cards and project context', () => {
		const { body } = render(UpdatePage, { props: { data: baseData } });
		expect(body).toContain('Release Channel');
		expect(body).toContain('Manual');
		expect(body).toContain('Machine Restart');
		expect(body).toContain('Project');
		expect(body).toContain('my-project');
		expect(body).toContain('/home/user/my-project');
	});

	it('renders the run update maintenance guidance by default', () => {
		const { body } = render(UpdatePage, { props: { data: baseData } });
		expect(body).toContain('Plan a controlled runtime update');
		expect(body).toContain('Update workflow is not connected to a backend yet.');
		expect(body).toContain('Check Release Notes');
		expect(body).toContain('Run Update');
	});

	it('renders the restart report guidance when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=restart-report' };
		const { body } = render(UpdatePage, { props: { data } });
		expect(body).toContain('Restart report');
		expect(body).toContain('No restart report captured yet.');
		expect(body).toContain('Apply with restart currently lives in Config.');
	});
});
