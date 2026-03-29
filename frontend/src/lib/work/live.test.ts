import { describe, expect, it, vi } from 'vitest';
import { loadLiveWorkSurface, shouldRefreshWorkSurface } from './live';

describe('work live helpers', () => {
	it('refreshes the work surface for graph-affecting events but not streaming deltas', () => {
		expect(shouldRefreshWorkSurface('turn_delta')).toBe(false);
		expect(shouldRefreshWorkSurface('run_updated')).toBe(true);
		expect(shouldRefreshWorkSurface('tool_log_recorded')).toBe(true);
	});

	it('loads detail and falls back to the inspector seed when the requested node is unavailable', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			const url = String(input);

			if (url === '/api/work/run-123') {
				return new Response(
					JSON.stringify({
						run: {
							id: 'run-123',
							short_id: 'run-123',
							objective_text: 'Fix auth regression',
							trigger_label: 'Chat',
							status: 'active',
							status_label: 'Active',
							status_class: 'is-active',
							state_label: 'Streaming',
							started_at_label: 'Started 5 minutes ago',
							last_activity_label: 'Last event just now',
							model_display: 'gpt-5.4',
							token_summary: '1.2K tokens',
							event_count: 8,
							turn_count: 2,
							stream_url: '/api/work/run-123/events',
							graph_url: '/api/work/run-123/graph',
							node_detail_url_template: '/api/work/run-123/nodes/{node_id}',
							dismissible: false
						},
						graph: {
							root_run_id: 'run-123',
							headline: 'Fix auth regression',
							summary: {
								total: 1,
								pending: 0,
								active: 1,
								needs_approval: 0,
								completed: 0,
								failed: 0,
								interrupted: 0,
								root_status: 'active'
							},
							nodes: [],
							edges: [],
							active_path: []
						},
						inspector_seed: {
							id: 'run-seed',
							agent_id: 'researcher',
							status: 'needs_approval'
						}
					}),
					{ status: 200, headers: { 'content-type': 'application/json' } }
				);
			}

			if (url === '/api/work/run-123/nodes/run-requested') {
				throw new Error('requested node unavailable');
			}

			if (url === '/api/work/run-123/nodes/run-seed') {
				return new Response(
					JSON.stringify({
						id: 'run-seed',
						short_id: 'seed',
						parent_run_id: 'run-123',
						parent_short_id: 'run-123',
						agent_id: 'researcher',
						status: 'needs_approval',
						status_label: 'needs approval',
						status_class: 'is-approval',
						model_display: 'gpt-5.4-mini',
						token_summary: '400 tokens',
						token_exact_summary: '200 input / 200 output',
						started_at_label: 'Started 3 minutes ago',
						last_activity_label: 'Last activity 1 minute ago',
						task: {
							plain_text: 'Inspect authentication logs',
							preview_text: 'Inspect authentication logs',
							has_overflow: false
						},
						output: {
							plain_text: 'Approval required before shell command can continue.',
							preview_text: 'Approval required before shell command can continue.',
							has_overflow: false
						},
						chain: {
							path: [],
							children: []
						}
					}),
					{ status: 200, headers: { 'content-type': 'application/json' } }
				);
			}

			throw new Error(`unexpected url: ${url}`);
		});

		const result = await loadLiveWorkSurface(fetcher, 'run-123', 'run-requested');

		expect(fetcher).toHaveBeenNthCalledWith(1, '/api/work/run-123', expect.any(Object));
		expect(fetcher).toHaveBeenNthCalledWith(
			2,
			'/api/work/run-123/nodes/run-requested',
			expect.any(Object)
		);
		expect(fetcher).toHaveBeenNthCalledWith(
			3,
			'/api/work/run-123/nodes/run-seed',
			expect.any(Object)
		);
		expect(result.detail.run.id).toBe('run-123');
		expect(result.nodeDetail?.id).toBe('run-seed');
		expect(result.inspectorNodeID).toBe('run-seed');
	});
});
