import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch, search = ''): Parameters<typeof load>[0] {
	return {
		fetch: fetcher,
		url: new URL(`http://localhost/sessions${search}`)
	} as unknown as Parameters<typeof load>[0];
}

describe('sessions load', () => {
	it('loads sessions, summary, filters, paging, runtime connectors, and history when requested', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			const url = String(input);

			if (url === '/api/conversations?role=worker&status=active') {
				return new Response(
					JSON.stringify({
						summary: {
							session_count: 1,
							connector_count: 1,
							terminal_deliveries: 0
						},
						filters: {
							query: '',
							agent_id: '',
							role: 'worker',
							status: 'active',
							connector_id: '',
							binding: ''
						},
						sessions: [
							{
								id: 'sess-1',
								conversation_id: 'conv-1',
								agent_id: 'front',
								role_label: 'User',
								status_label: 'Active',
								updated_at_label: '2 min ago'
							}
						],
						paging: {
							has_next: true,
							has_prev: false,
							next_url:
								'/api/conversations?status=active&role=worker&cursor=cursor-next&direction=next'
						},
						health: [],
						runtime_connectors: [
							{
								connector_id: 'telegram',
								state: 'active',
								state_label: 'Active',
								state_class: 'is-success',
								summary: 'Connected',
								checked_at_label: '1 min ago',
								restart_suggested: false
							}
						]
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			if (url === '/api/history?q=repair&status=failed&scope=all&limit=10') {
				return new Response(
					JSON.stringify({
						summary: {
							run_count: 2,
							completed_runs: 1,
							recovery_runs: 1,
							approval_events: 1,
							delivery_outcomes: 1
						},
						filters: {
							query: 'repair',
							status: 'failed',
							scope: 'all',
							limit: 10
						},
						paging: {
							has_next: false,
							has_prev: false
						},
						runs: [
							{
								root: {
									id: 'run-123',
									objective: 'Repair connector backlog',
									agent_id: 'front',
									status: 'failed',
									status_label: 'Failed',
									status_class: 'is-error',
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
								child_count_label: 'No child runs',
								blocker_label: 'Needs operator review',
								has_children: false
							}
						],
						approvals: [
							{
								id: 'approval-1',
								run_id: 'run-123',
								tool_name: 'apply_patch',
								status: 'approved',
								status_label: 'Approved',
								resolved_by: 'operator',
								resolved_at_label: '1 min ago'
							}
						],
						deliveries: [
							{
								id: 'delivery-1',
								run_id: 'run-123',
								connector_id: 'telegram',
								chat_id: 'chat-1',
								status: 'terminal',
								status_label: 'Terminal',
								attempts_label: '2 attempts',
								last_attempt_at_label: 'just now',
								message_preview: 'Retry exhausted'
							}
						]
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			throw new Error(`unexpected url: ${url}`);
		});

		const result = await load(
			makeLoadEvent(
				fetcher,
				'?status=active&role=worker&tab=history&history_q=repair&history_status=failed&history_scope=all&history_limit=10'
			)
		);

		if (!result) {
			throw new Error('expected sessions load to return data');
		}

		expect(fetcher).toHaveBeenNthCalledWith(
			1,
			'/api/conversations?role=worker&status=active',
			expect.any(Object)
		);
		expect(fetcher).toHaveBeenNthCalledWith(
			2,
			'/api/history?q=repair&status=failed&scope=all&limit=10',
			expect.any(Object)
		);
		expect(result.sessions.summary.session_count).toBe(1);
		expect(result.sessions.filters.role).toBe('worker');
		expect(result.sessions.items).toHaveLength(1);
		expect(result.sessions.runtimeConnectors).toHaveLength(1);
		expect(result.sessions.paging.nextHref).toBe(
			'/sessions?status=active&role=worker&cursor=cursor-next&direction=next&tab=history'
		);
		expect(result.sessions.history.summary.run_count).toBe(2);
		expect(result.sessions.history.filters.query).toBe('repair');
		expect(result.sessions.history.filters.status).toBe('failed');
		expect(result.sessions.history.filters.limit).toBe(10);
		expect(result.sessions.history.runs).toHaveLength(1);
		expect(result.sessions.history.approvals[0]?.tool_name).toBe('apply_patch');
		expect(result.sessions.history.deliveries[0]?.connector_id).toBe('telegram');
	});

	it('does not load history when the history tab is not requested', async () => {
		const fetcher = vi.fn<typeof fetch>(
			async () =>
				new Response(
					JSON.stringify({
						summary: {
							session_count: 1,
							connector_count: 1,
							terminal_deliveries: 0
						},
						filters: {
							query: '',
							agent_id: '',
							role: '',
							status: '',
							connector_id: '',
							binding: ''
						},
						sessions: [],
						paging: {
							has_next: false,
							has_prev: false
						},
						health: [],
						runtime_connectors: []
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				)
		);

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected sessions load to return data');
		}

		expect(fetcher).toHaveBeenCalledTimes(1);
		expect(result.sessions.history.summary.run_count).toBe(0);
	});

	it('loads selected session detail and delivery queue filters when a session query is provided', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			const url = String(input);

			if (url === '/api/conversations') {
				return new Response(
					JSON.stringify({
						summary: {
							session_count: 1,
							connector_count: 1,
							terminal_deliveries: 1
						},
						filters: {
							query: '',
							agent_id: '',
							role: '',
							status: '',
							connector_id: '',
							binding: ''
						},
						sessions: [
							{
								id: 'sess-1',
								conversation_id: 'conv-1',
								agent_id: 'front',
								role_label: 'User',
								status_label: 'Active',
								updated_at_label: '2 min ago'
							}
						],
						paging: {
							has_next: false,
							has_prev: false
						},
						health: [],
						runtime_connectors: []
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			if (url === '/api/conversations/sess-1') {
				return new Response(
					JSON.stringify({
						session: {
							id: 'sess-1',
							agent_id: 'front',
							role_label: 'User',
							status_label: 'Active'
						},
						messages: [],
						route: {
							id: 'route-1',
							connector_id: 'telegram',
							external_id: 'ext-1',
							thread_id: 'thread-1',
							status_label: 'Active',
							created_at_label: 'just now'
						},
						active_run_id: 'run-1',
						deliveries: [
							{
								id: 'delivery-1',
								connector_id: 'telegram',
								chat_id: 'chat-1',
								message: { plain_text: 'Retry exhausted', html: '<p>Retry exhausted</p>' },
								status: 'terminal',
								status_label: 'Terminal',
								attempts_label: '2 attempts'
							}
						],
						delivery_failures: []
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			if (url === '/api/deliveries?session_id=sess-1&q=follow-up&status=terminal&limit=25') {
				return new Response(
					JSON.stringify({
						deliveries: [
							{
								ID: 'delivery-queue-1',
								RunID: 'run-queue-1',
								SessionID: 'sess-1',
								ConversationID: 'conv-1',
								ConnectorID: 'telegram',
								ChatID: 'chat-1',
								MessageText: 'Queued follow-up',
								Status: 'terminal',
								Attempts: 2
							}
						],
						has_next: true,
						has_prev: false,
						next_cursor: 'delivery-next'
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			throw new Error(`unexpected url: ${url}`);
		});

		const result = await load(
			makeLoadEvent(
				fetcher,
				'?tab=overrides&session=sess-1&delivery_q=follow-up&delivery_status=terminal&delivery_limit=25'
			)
		);

		if (!result) {
			throw new Error('expected sessions load to return data');
		}

		expect(fetcher).toHaveBeenNthCalledWith(1, '/api/conversations', expect.any(Object));
		expect(fetcher).toHaveBeenNthCalledWith(2, '/api/conversations/sess-1', expect.any(Object));
		expect(fetcher).toHaveBeenNthCalledWith(
			3,
			'/api/deliveries?session_id=sess-1&q=follow-up&status=terminal&limit=25',
			expect.any(Object)
		);
		expect(result.sessions.selectedDetail?.session.id).toBe('sess-1');
		expect(result.sessions.selectedDetail?.route?.id).toBe('route-1');
		expect(result.sessions.deliveryQueue.filters).toEqual({
			query: 'follow-up',
			status: 'terminal',
			limit: 25
		});
		expect(result.sessions.deliveryQueue.items).toHaveLength(1);
		expect(result.sessions.deliveryQueue.items[0]).toMatchObject({
			id: 'delivery-queue-1',
			run_id: 'run-queue-1',
			connector_id: 'telegram',
			status_label: 'Terminal',
			attempts_label: '2 attempts',
			message_preview: 'Queued follow-up'
		});
		expect(result.sessions.deliveryQueue.paging.nextHref).toBe(
			'/sessions?tab=overrides&session=sess-1&delivery_q=follow-up&delivery_status=terminal&delivery_limit=25&delivery_cursor=delivery-next&delivery_direction=next'
		);
	});

	it('returns empty sessions data when the request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected sessions load to return fallback data');
		}

		expect(result).toEqual({
			sessions: {
				summary: {
					session_count: 0,
					connector_count: 0,
					terminal_deliveries: 0
				},
				filters: {
					query: '',
					agent_id: '',
					role: '',
					status: '',
					connector_id: '',
					binding: ''
				},
				items: [],
				paging: { has_next: false, has_prev: false, nextHref: undefined, prevHref: undefined },
				runtimeConnectors: [],
				selectedDetail: null,
				deliveryQueue: {
					filters: {
						query: '',
						status: '',
						limit: 50
					},
					items: [],
					paging: {
						has_next: false,
						has_prev: false,
						nextHref: undefined,
						prevHref: undefined
					}
				},
				history: {
					summary: {
						run_count: 0,
						completed_runs: 0,
						recovery_runs: 0,
						approval_events: 0,
						delivery_outcomes: 0
					},
					filters: {
						query: '',
						status: '',
						scope: 'all',
						limit: 0
					},
					paging: { has_next: false, has_prev: false },
					runs: [],
					approvals: [],
					deliveries: []
				}
			}
		});
	});
});
