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
		summary: {
			total_schedules: 3,
			enabled_schedules: 2,
			due_schedules: 1,
			active_occurrences: 1,
			next_wake_at_label: '2026-03-30 02:00 UTC'
		},
		health: {
			invalid_schedules: 1,
			stuck_dispatching: 0,
			missing_next_run: 1
		},
		schedules: [],
		openOccurrences: [],
		recentOccurrences: []
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

	it('renders scheduler summary and health cards', () => {
		const { body } = render(CronPage, { props: { data: baseData } });
		expect(body).toContain('3');
		expect(body).toContain('2026-03-30 02:00 UTC');
		expect(body).toContain('1 invalid');
		expect(body).toContain('1 missing next wake');
	});

	it('renders the real editor tab when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=editor' };
		const { body } = render(CronPage, { props: { data } });
		expect(body).toContain('Create Job');
		expect(body).toContain('Objective');
		expect(body).toContain('Schedule type');
		expect(body).toContain('Cron expression');
		expect(body).toContain('Timezone');
	});

	it('renders open and recent run lanes on the runs tab', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=runs',
			cron: {
				...baseData.cron,
				openOccurrences: [
					{
						id: 'occ-open',
						schedule_id: 'sched-1',
						schedule_name: 'Daily digest',
						status: 'active',
						status_label: 'Active',
						status_class: 'is-active',
						slot_at_label: 'Today 09:00',
						updated_at_label: 'just now'
					}
				],
				recentOccurrences: [
					{
						id: 'occ-recent',
						schedule_id: 'sched-2',
						schedule_name: 'Repository sweep',
						status: 'completed',
						status_label: 'Completed',
						status_class: 'is-active',
						slot_at_label: 'Today 03:00',
						updated_at_label: '2 min ago'
					}
				]
			}
		};
		const { body } = render(CronPage, { props: { data } });
		expect(body).toContain('Open runs');
		expect(body).toContain('Recent runs');
		expect(body).toContain('Daily digest');
		expect(body).toContain('Repository sweep');
	});

	it('renders a schedule row when schedules are provided', () => {
		const data = {
			...baseData,
			cron: {
				...baseData.cron,
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
				openOccurrences: [],
				recentOccurrences: []
			}
		};
		const { body } = render(CronPage, { props: { data } });
		expect(body).toContain('Daily digest');
	});
});
