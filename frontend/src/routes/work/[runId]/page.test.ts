import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import WorkRunPage from './+page.svelte';

describe('Work run detail page', () => {
	it('shows that the live stream is attached to the run detail surface', () => {
		const { body } = render(WorkRunPage, {
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
					currentPath: '/work/run-work-root',
					currentSearch: '',
					work: {
						run: {
							id: 'run-work-root',
							short_id: 'workroot',
							objective_text: 'Review the repo',
							trigger_label: 'GistClaw',
							status: 'active',
							status_label: 'active',
							status_class: 'is-active',
							state_label: '1 task waiting on you.',
							started_at_label: '5m ago',
							last_activity_label: '1m ago',
							model_display: 'gpt-5.4',
							token_summary: '2k in / 900 out',
							event_count: 6,
							turn_count: 2,
							stream_url: '/api/work/run-work-root/events',
							graph_url: '/api/work/run-work-root/graph',
							node_detail_url_template: '/api/work/run-work-root/nodes/__RUN_ID__'
						},
						graph: {
							root_run_id: 'run-work-root',
							headline: '1 task waiting on you.',
							summary: {
								total: 2,
								pending: 0,
								active: 1,
								needs_approval: 1,
								completed: 0,
								failed: 0,
								interrupted: 0,
								root_status: 'active'
							},
							active_path: ['run-work-root', 'run-work-child'],
							nodes: [
								{
									id: 'run-work-root',
									short_id: 'workroot',
									short_label: 'workroot',
									parent_run_id: '',
									agent_id: 'assistant',
									objective: 'Review the repo',
									objective_preview: 'Review the repo',
									status: 'active',
									status_label: 'active',
									status_class: 'is-active',
									kind: 'root',
									lane_id: 'lead',
									model_display: 'gpt-5.4',
									token_summary: '2k in / 900 out',
									time_label: '5m ago',
									started_at_label: '5m ago',
									updated_at_label: '1m ago',
									depth: 0,
									is_root: true,
									is_active_path: true,
									child_count: 1
								}
							],
							edges: []
						},
						inspector_seed: {
							id: 'run-work-root',
							agent_id: 'assistant',
							status: 'active'
						}
					}
				}
			}
		});

		expect(body).toContain('Live stream attached');
		expect(body).toContain('/api/work/run-work-root/events');
	});
});
