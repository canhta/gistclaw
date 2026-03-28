import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import AutomatePage from './+page.svelte';

describe('Automate page', () => {
	it('renders the scheduler board and action controls from the future-work surface', () => {
		const { body } = render(AutomatePage, {
			props: {
				data: {
					auth: {
						authenticated: true,
						password_configured: true,
						setup_required: false
					},
					onboarding: {
						completed: true,
						entry_href: '/work'
					},
					project: {
						active_id: 'proj-primary',
						active_name: 'starter-project',
						active_path: '/tmp/starter-project'
					},
					navigation: [{ id: 'automate', label: 'Automate', href: '/automate' }],
					currentPath: '/automate',
					currentSearch: '',
					automate: {
						summary: {
							total_schedules: 2,
							enabled_schedules: 1,
							due_schedules: 0,
							active_occurrences: 1,
							next_wake_at_label: 'in 53m'
						},
						health: {
							invalid_schedules: 0,
							stuck_dispatching: 0,
							missing_next_run: 0
						},
						schedules: [
							{
								id: 'sched-review',
								name: 'Repo review',
								objective: 'Audit the repository every two hours.',
								kind: 'every',
								kind_label: 'Every',
								cadence_label: 'Every 2h from 08:00 UTC',
								enabled: true,
								enabled_label: 'Enabled',
								status_label: 'Healthy',
								status_class: 'is-active',
								next_run_at_label: 'in 53m',
								last_run_at_label: '2h ago',
								last_error: '',
								project_id: 'proj-primary',
								cwd: '/tmp/starter-project',
								consecutive_failures: 0,
								schedule_error_count: 0
							}
						],
						open_occurrences: [
							{
								id: 'occ-open',
								schedule_id: 'sched-review',
								schedule_name: 'Repo review',
								status: 'needs_approval',
								status_label: 'Needs approval',
								status_class: 'is-approval',
								slot_at_label: '5m ago',
								updated_at_label: '5m ago',
								run_id: 'run-open',
								conversation_id: 'conv-open',
								error: '',
								skip_reason: ''
							}
						],
						recent_occurrences: [
							{
								id: 'occ-done',
								schedule_id: 'sched-review',
								schedule_name: 'Repo review',
								status: 'completed',
								status_label: 'Completed',
								status_class: 'is-active',
								slot_at_label: '2h ago',
								updated_at_label: '90m ago',
								run_id: 'run-done',
								conversation_id: 'conv-done',
								error: '',
								skip_reason: ''
							}
						]
					}
				}
			}
		});

		expect(body).toContain('Keep recurring work moving without checking in manually');
		expect(body).toContain(
			'Set up repeat work, see what runs next, and fix schedules before they slip.'
		);
		expect(body).toContain('Repo review');
		expect(body).toContain('Every 2h from 08:00 UTC');
		expect(body).toContain('Run now');
		expect(body).toContain('Create schedule');
		expect(body).toContain('/work/run-open');
	});

	it('normalizes empty next-run copy into user-facing language', () => {
		const { body } = render(AutomatePage, {
			props: {
				data: {
					auth: {
						authenticated: true,
						password_configured: true,
						setup_required: false
					},
					onboarding: {
						completed: true,
						entry_href: '/work'
					},
					project: {
						active_id: 'proj-primary',
						active_name: 'starter-project',
						active_path: '/tmp/starter-project'
					},
					navigation: [{ id: 'automate', label: 'Automate', href: '/automate' }],
					currentPath: '/automate',
					currentSearch: '',
					automate: {
						summary: {
							total_schedules: 0,
							enabled_schedules: 0,
							due_schedules: 0,
							active_occurrences: 0,
							next_wake_at_label: 'No wake scheduled'
						},
						health: {
							invalid_schedules: 0,
							stuck_dispatching: 0,
							missing_next_run: 0
						},
						schedules: [],
						open_occurrences: [],
						recent_occurrences: []
					}
				}
			}
		});

		expect(body).toContain('No schedule yet');
		expect(body).not.toContain('No wake scheduled');
	});
});
