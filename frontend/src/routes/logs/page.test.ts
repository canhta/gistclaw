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

	it('renders the logs summary cards and project context', () => {
		const { body } = render(LogsPage, { props: { data: baseData } });
		expect(body).toContain('Tail Status');
		expect(body).toContain('Offline');
		expect(body).toContain('Default Window');
		expect(body).toContain('Project');
		expect(body).toContain('my-project');
		expect(body).toContain('/home/user/my-project');
	});

	it('renders the live tail placeholder workflow by default', () => {
		const { body } = render(LogsPage, { props: { data: baseData } });
		expect(body).toContain('Live tail is waiting on the runtime log stream.');
		expect(body).toContain('Start Tail');
		expect(body).toContain('Auto-follow');
		expect(body).toContain('No log stream available yet.');
	});

	it('renders the filter workbench when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=filters' };
		const { body } = render(LogsPage, { props: { data } });
		expect(body).toContain('Filter toolbar');
		expect(body).toContain('Search logs');
		expect(body).toContain('Level');
		expect(body).toContain('Time Range');
	});

	it('renders export actions when the export tab is selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=export' };
		const { body } = render(LogsPage, { props: { data } });
		expect(body).toContain('Export buffer');
		expect(body).toContain('Download Current Buffer');
		expect(body).toContain('Copy JSONL');
		expect(body).toContain('Export depends on the same tail backend.');
	});
});
