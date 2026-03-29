import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch, search = ''): Parameters<typeof load>[0] {
	return {
		fetch: fetcher,
		url: new URL(`http://localhost/chat${search}`)
	} as unknown as Parameters<typeof load>[0];
}

describe('chat load', () => {
	it('loads queue summary, selected run detail, and mapped paging links', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			const url = String(input);

			if (url === '/api/work?limit=1') {
				return new Response(
					JSON.stringify({
						active_project_name: 'repo',
						active_project_path: '/workspace/repo',
						queue_strip: {
							headline: '1 active run',
							root_runs: 1,
							worker_runs: 2,
							recovery_runs: 0,
							summary: {
								total: 1,
								pending: 0,
								active: 1,
								needs_approval: 0,
								completed: 0,
								failed: 0,
								interrupted: 0,
								root_status: 'active'
							}
						},
						paging: {
							has_next: true,
							has_prev: false,
							next_url: '/api/work?cursor=next-cursor&direction=next'
						},
						clusters: [
							{
								root: {
									id: 'run-123',
									objective: 'Fix auth regression',
									agent_id: 'front',
									status: 'active',
									status_label: 'Active',
									status_class: 'is-active',
									model_display: 'gpt-5.4',
									token_summary: '1.2K tokens',
									started_at_short: '10:00',
									started_at_exact: '2026-03-29 10:00',
									started_at_iso: '2026-03-29T10:00:00Z',
									last_activity_short: '10:05',
									last_activity_exact: '2026-03-29 10:05',
									last_activity_iso: '2026-03-29T10:05:00Z',
									depth: 0
								},
								children: [],
								child_count: 0,
								child_count_label: '',
								blocker_label: '',
								has_children: false
							}
						]
					}),
					{ status: 200, headers: { 'content-type': 'application/json' } }
				);
			}

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
						}
					}),
					{ status: 200, headers: { 'content-type': 'application/json' } }
				);
			}

			throw new Error(`unexpected url: ${url}`);
		});

		const result = await load(makeLoadEvent(fetcher, '?limit=1&tab=run-events&run=run-123'));

		if (!result) {
			throw new Error('expected chat load to return data');
		}

		expect(fetcher).toHaveBeenNthCalledWith(1, '/api/work?limit=1', expect.any(Object));
		expect(fetcher).toHaveBeenNthCalledWith(2, '/api/work/run-123', expect.any(Object));
		expect(result.chat.queue.headline).toBe('1 active run');
		expect(result.chat.selectedRunID).toBe('run-123');
		expect(result.chat.detail?.run.objective_text).toBe('Fix auth regression');
		expect(result.chat.paging.nextHref).toBe(
			'/chat?cursor=next-cursor&direction=next&tab=run-events&run=run-123'
		);
	});

	it('returns queue and runs even when the selected run detail fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			const url = String(input);

			if (url === '/api/work') {
				return new Response(
					JSON.stringify({
						active_project_name: 'repo',
						active_project_path: '/workspace/repo',
						queue_strip: {
							headline: 'No active runs',
							root_runs: 0,
							worker_runs: 0,
							recovery_runs: 0,
							summary: {
								total: 0,
								pending: 0,
								active: 0,
								needs_approval: 0,
								completed: 0,
								failed: 0,
								interrupted: 0,
								root_status: 'idle'
							}
						},
						paging: { has_next: false, has_prev: false },
						clusters: [
							{
								root: {
									id: 'run-123',
									objective: 'Fix auth regression',
									agent_id: 'front',
									status: 'active',
									status_label: 'Active',
									status_class: 'is-active',
									model_display: 'gpt-5.4',
									token_summary: '1.2K tokens',
									started_at_short: '10:00',
									started_at_exact: '2026-03-29 10:00',
									started_at_iso: '2026-03-29T10:00:00Z',
									last_activity_short: '10:05',
									last_activity_exact: '2026-03-29 10:05',
									last_activity_iso: '2026-03-29T10:05:00Z',
									depth: 0
								},
								children: [],
								child_count: 0,
								child_count_label: '',
								blocker_label: '',
								has_children: false
							}
						]
					}),
					{ status: 200, headers: { 'content-type': 'application/json' } }
				);
			}

			throw new Error('detail failed');
		});

		const result = await load(makeLoadEvent(fetcher, '?run=run-123'));

		if (!result) {
			throw new Error('expected chat load to return fallback detail state');
		}

		expect(result.chat.runs).toHaveLength(1);
		expect(result.chat.selectedRunID).toBe('run-123');
		expect(result.chat.detail).toBeNull();
	});
});
