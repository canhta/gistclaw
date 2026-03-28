import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import RunGraph from './RunGraph.svelte';

describe('RunGraph', () => {
	it('renders the active path and run nodes for a work detail surface', () => {
		const { body } = render(RunGraph, {
			props: {
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
						},
						{
							id: 'run-work-child',
							short_id: 'workchild',
							short_label: 'workchild',
							parent_run_id: 'run-work-root',
							agent_id: 'researcher',
							objective: 'Inspect docs',
							objective_preview: 'Inspect docs',
							status: 'needs_approval',
							status_label: 'needs approval',
							status_class: 'is-approval',
							kind: 'worker',
							lane_id: 'researcher',
							model_display: 'gpt-5.4-mini',
							token_summary: '600 in / 300 out',
							time_label: '4m ago',
							started_at_label: '4m ago',
							updated_at_label: '1m ago',
							depth: 1,
							is_root: false,
							is_active_path: true,
							child_count: 0
						}
					],
					edges: [
						{
							id: 'run-work-root->run-work-child:delegates',
							from: 'run-work-root',
							to: 'run-work-child',
							kind: 'delegates',
							label: 'delegates'
						}
					]
				},
				inspectorSeedID: 'run-work-child'
			}
		});

		expect(body).toContain('1 task waiting on you.');
		expect(body).toContain('Review the repo');
		expect(body).toContain('Inspect docs');
		expect(body).toContain('run-work-child');
	});
});
