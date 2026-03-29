import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ScheduleRow from './ScheduleRow.svelte';
import type { AutomateScheduleResponse } from '$lib/types/api';

const enabledSchedule: AutomateScheduleResponse = {
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
};

const disabledSchedule: AutomateScheduleResponse = {
	...enabledSchedule,
	id: 'sched-2',
	name: 'Weekly report',
	enabled: false,
	enabled_label: 'Disabled'
};

describe('ScheduleRow', () => {
	it('renders the schedule name', () => {
		const { body } = render(ScheduleRow, { props: { schedule: enabledSchedule } });
		expect(body).toContain('Daily digest');
	});

	it('renders the cadence label', () => {
		const { body } = render(ScheduleRow, { props: { schedule: enabledSchedule } });
		expect(body).toContain('Every day at 09:00');
	});

	it('renders the next run label', () => {
		const { body } = render(ScheduleRow, { props: { schedule: enabledSchedule } });
		expect(body).toContain('in 3h');
	});

	it('renders Enabled badge when enabled', () => {
		const { body } = render(ScheduleRow, { props: { schedule: enabledSchedule } });
		expect(body).toContain('Enabled');
	});

	it('renders Disabled badge when disabled', () => {
		const { body } = render(ScheduleRow, { props: { schedule: disabledSchedule } });
		expect(body).toContain('Disabled');
	});

	it('renders the status label', () => {
		const { body } = render(ScheduleRow, { props: { schedule: enabledSchedule } });
		expect(body).toContain('OK');
	});
});
