import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import HistoryPage from './+page.svelte';

describe('History page', () => {
	it('renders run evidence, operator interventions, and delivery outcomes', () => {
		const { body } = render(HistoryPage, {
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
					navigation: [{ id: 'history', label: 'History', href: '/history' }],
					currentPath: '/history',
					currentSearch: '',
					history: {
						summary: {
							run_count: 4,
							completed_runs: 2,
							recovery_runs: 1,
							approval_events: 2,
							delivery_outcomes: 1
						},
						filters: {
							query: '',
							status: '',
							scope: 'all',
							limit: 20
						},
						paging: {
							has_next: false,
							has_prev: false
						},
						runs: [
							{
								root: {
									id: 'run-history-complete',
									objective: 'Inspect repository health',
									agent_id: 'assistant',
									status: 'completed',
									status_label: 'Completed',
									status_class: 'is-active',
									model_display: 'GPT-5.4',
									token_summary: '2.4K tokens',
									started_at_short: '2026-03-25 10:00 UTC',
									started_at_exact: '2026-03-25 10:00:00 UTC',
									started_at_iso: '2026-03-25T10:00:00Z',
									last_activity_short: '2026-03-25 10:04 UTC',
									last_activity_exact: '2026-03-25 10:04:00 UTC',
									last_activity_iso: '2026-03-25T10:04:00Z',
									depth: 0
								},
								children: [],
								child_count: 0,
								child_count_label: '0 sub-agents',
								blocker_label: 'Finished cleanly.',
								has_children: false
							}
						],
						approvals: [
							{
								id: 'approval-1',
								run_id: 'run-history-complete',
								tool_name: 'apply_patch',
								status: 'approved',
								status_label: 'Approved',
								resolved_by: 'operator',
								resolved_at_label: '2026-03-25 10:15:00 UTC'
							}
						],
						deliveries: [
							{
								id: 'delivery-1',
								run_id: 'run-route',
								connector_id: 'telegram',
								chat_id: 'chat-1',
								status: 'terminal',
								status_label: 'Terminal',
								attempts_label: '3 attempts',
								last_attempt_at_label: '2026-03-25 10:20:00 UTC',
								message_preview: 'Retry this message.'
							}
						]
					}
				}
			}
		});

		expect(body).toContain('See what happened before you decide what to do next');
		expect(body).toContain(
			'Review finished runs, approvals, and delivery results so the next step starts from evidence.'
		);
		expect(body).toContain('Inspect repository health');
		expect(body).toContain('apply_patch');
		expect(body).toContain('3 attempts');
		expect(body).toContain('/work/run-history-complete');
	});
});
