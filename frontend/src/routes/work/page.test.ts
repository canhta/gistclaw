import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import WorkPage from './+page.svelte';

describe('Work page', () => {
	it('renders queue strip, project context, and run entry points', () => {
		const { body } = render(WorkPage, {
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
					navigation: [
						{ id: 'work', label: 'Work', href: '/work' },
						{ id: 'recover', label: 'Recover', href: '/recover' }
					],
					currentPath: '/work',
					currentSearch: '',
					work: {
						active_project_name: 'starter-project',
						active_project_path: '/tmp/starter-project',
						queue_strip: {
							headline: 'Some work is waiting on you.',
							root_runs: 1,
							worker_runs: 1,
							recovery_runs: 1,
							summary: {
								total: 1,
								pending: 0,
								active: 0,
								needs_approval: 1,
								completed: 0,
								failed: 0,
								interrupted: 0,
								root_status: 'needs_approval'
							}
						},
						paging: {
							next_url: '',
							prev_url: '',
							has_next: false,
							has_prev: false
						},
						clusters: [
							{
								root: {
									id: 'run-work-root',
									objective: 'Review the repo',
									agent_id: 'assistant',
									status: 'needs_approval',
									status_label: 'needs approval',
									status_class: 'is-approval',
									model_display: 'gpt-5.4',
									token_summary: '2k in / 900 out',
									started_at_short: '5m ago',
									started_at_exact: '2026-03-28 15:45',
									started_at_iso: '2026-03-28T15:45:00Z',
									last_activity_short: '1m ago',
									last_activity_exact: '2026-03-28 15:49',
									last_activity_iso: '2026-03-28T15:49:00Z',
									depth: 0
								},
								children: [
									{
										id: 'run-work-child',
										objective: 'Inspect docs',
										agent_id: 'researcher',
										status: 'needs_approval',
										status_label: 'needs approval',
										status_class: 'is-approval',
										model_display: 'gpt-5.4-mini',
										token_summary: '600 in / 300 out',
										started_at_short: '4m ago',
										started_at_exact: '2026-03-28 15:46',
										started_at_iso: '2026-03-28T15:46:00Z',
										last_activity_short: '1m ago',
										last_activity_exact: '2026-03-28 15:49',
										last_activity_iso: '2026-03-28T15:49:00Z',
										depth: 1
									}
								],
								child_count: 1,
								child_count_label: '1 sub-agent',
								blocker_label: 'researcher waiting on approval',
								has_children: true
							}
						]
					}
				}
			}
		});

		expect(body).toContain('Some work is waiting on you.');
		expect(body).toContain('starter-project');
		expect(body).toContain('/tmp/starter-project');
		expect(body).toContain('Review the repo');
		expect(body).toContain('researcher waiting on approval');
		expect(body).toContain('/work/run-work-root');
		expect(body).toContain('Open run graph');
		expect(body).toContain('Describe the work you want GistClaw to handle next');
	});
});
