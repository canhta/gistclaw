import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import LogsPage from './+page.svelte';

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: [{ id: 'logs', label: 'Logs', href: '/logs' }],
	onboarding: null,
	currentPath: '/logs',
	currentSearch: '',
	logsLoadError: '',
	logs: {
		summary: {
			buffered_entries: 18,
			visible_entries: 3,
			error_entries: 1,
			warning_entries: 1
		},
		filters: {
			query: '',
			level: 'all',
			source: 'all',
			limit: 200
		},
		sources: ['runtime', 'scheduler', 'web'],
		stream_url: '/api/logs/stream?limit=200',
		entries: [
			{
				id: 1,
				source: 'runtime',
				level: 'info',
				level_label: 'Info',
				message: 'startup_reconciled reconciled_runs=1 expired_approvals=0',
				raw: 'runtime info startup_reconciled reconciled_runs=1 expired_approvals=0',
				created_at_label: '2026-03-29 10:00:00 UTC'
			},
			{
				id: 2,
				source: 'web',
				level: 'warn',
				level_label: 'Warn',
				message: 'panic method=GET path=/api/debug err=boom',
				raw: 'web warn panic method=GET path=/api/debug err=boom',
				created_at_label: '2026-03-29 10:00:02 UTC'
			}
		]
	}
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
		expect(body).toContain('Buffered Lines');
		expect(body).toContain('18');
		expect(body).toContain('Visible Window');
		expect(body).toContain('3');
		expect(body).toContain('Project');
		expect(body).toContain('my-project');
		expect(body).toContain('/home/user/my-project');
	});

	it('renders the live tail workbench and current log lines by default', () => {
		const { body } = render(LogsPage, { props: { data: baseData } });
		expect(body).toContain('Live tail');
		expect(body).toContain('Pause Tail');
		expect(body).toContain('Auto-follow');
		expect(body).toContain('startup_reconciled reconciled_runs=1 expired_approvals=0');
		expect(body).toContain('panic method=GET path=/api/debug err=boom');
		expect(body).not.toContain('Live tail is waiting on the runtime log stream.');
	});

	it('renders the filter workbench when selected through search', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=filters&q=panic&level=warn&source=web&limit=100'
		};
		const { body } = render(LogsPage, { props: { data } });
		expect(body).toContain('Filter toolbar');
		expect(body).toContain('Search logs');
		expect(body).toContain('Level');
		expect(body).toContain('Source');
		expect(body).toContain('Apply Filters');
	});

	it('renders export actions when the export tab is selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=export' };
		const { body } = render(LogsPage, { props: { data } });
		expect(body).toContain('Export buffer');
		expect(body).toContain('Download JSONL');
		expect(body).toContain('Copy JSONL');
		expect(body).toContain('2 entries ready to hand off');
	});

	it('renders an honest load error instead of a synthetic empty board', () => {
		const data = {
			...baseData,
			logs: null,
			logsLoadError: 'Logs could not be loaded. Reload to retry.'
		};

		const { body } = render(LogsPage, { props: { data } });

		expect(body).toContain('Logs');
		expect(body).toContain('Logs could not be loaded. Reload to retry.');
		expect(body).toContain('Logs board unavailable');
		expect(body).not.toContain('Live tail');
		expect(body).not.toContain('Buffered Lines');
	});
});
