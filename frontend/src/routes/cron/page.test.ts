import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import CronPage from './+page.svelte';

const nav = [{ id: 'cron', label: 'Cron Jobs', href: '/cron' }];

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: nav,
	onboarding: null,
	currentPath: '/cron',
	currentSearch: '',
	cron: {
		schedules: [],
		occurrences: []
	}
};

describe('Cron page', () => {
	it('renders the Cron Jobs heading', () => {
		const { body } = render(CronPage, { props: { data: baseData } });
		expect(body).toContain('Cron Jobs');
	});

	it('renders Jobs, Runs, Editor tabs', () => {
		const { body } = render(CronPage, { props: { data: baseData } });
		expect(body).toContain('Jobs');
		expect(body).toContain('Runs');
		expect(body).toContain('Editor');
	});

	it('renders an empty state when no schedules exist', () => {
		const { body } = render(CronPage, { props: { data: baseData } });
		expect(body).toContain('No jobs scheduled');
	});

	it('renders a schedule row when schedules are provided', () => {
		const data = {
			...baseData,
			cron: {
				schedules: [
					{
						id: 'sched-1',
						name: 'Daily digest',
						objective: 'Send a daily summary',
						kind: 'cron',
						kind_label: 'Cron',
						cadence_label: 'Every day at 09:00',
						enabled: true,
						enabled_label: 'Enabled',
						status_label: 'OK',
						status_class: 'is-success',
						next_run_at_label: 'in 3h',
						last_run_at_label: '1 day ago',
						last_error: '',
						project_id: 'p1',
						cwd: '/home/user/project',
						consecutive_failures: 0,
						schedule_error_count: 0
					}
				],
				occurrences: []
			}
		};
		const { body } = render(CronPage, { props: { data } });
		expect(body).toContain('Daily digest');
	});
});
