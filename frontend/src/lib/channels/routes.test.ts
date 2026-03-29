import { describe, expect, it, vi } from 'vitest';
import { buildChannelRoutesHref, buildChannelRoutesSearch, loadChannelRoutes } from './routes';

describe('channel route helpers', () => {
	it('maps channel settings route filters to the routes api contract', () => {
		const params = new URLSearchParams({
			tab: 'settings',
			route_connector_id: 'telegram',
			route_status: 'all',
			route_q: 'chat-1',
			route_limit: '10'
		});

		expect(buildChannelRoutesSearch(params)).toBe(
			'connector_id=telegram&status=all&q=chat-1&limit=10'
		);
	});

	it('maps route paging back to the channels settings tab', () => {
		expect(
			buildChannelRoutesHref(
				'cursor-next',
				'next',
				'tab=settings&route_connector_id=telegram&route_status=all&route_q=chat-1&route_limit=10'
			)
		).toBe(
			'/channels?tab=settings&route_connector_id=telegram&route_status=all&route_q=chat-1&route_limit=10&route_cursor=cursor-next&route_direction=next'
		);
	});

	it('normalizes the routes directory payload', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			return new Response(
				JSON.stringify({
					routes: [
						{
							ID: 'route-1',
							SessionID: 'sess-1',
							ThreadID: 'thread-1',
							ConnectorID: 'telegram',
							AccountID: 'acct-1',
							ExternalID: 'chat-1',
							Status: 'inactive',
							CreatedAt: '2026-03-29T10:00:00Z',
							DeactivatedAt: '2026-03-29T11:00:00Z',
							DeactivationReason: 'deactivated',
							ReplacedByRouteID: '',
							ConversationID: 'conv-1',
							AgentID: 'assistant',
							Role: 'front'
						}
					],
					has_next: true,
					has_prev: false,
					next_cursor: 'cursor-next'
				}),
				{
					status: 200,
					headers: { 'content-type': 'application/json' }
				}
			);
		});

		const data = await loadChannelRoutes(
			fetcher,
			'connector_id=telegram&status=all&q=chat-1&limit=10',
			'tab=settings&route_connector_id=telegram&route_status=all&route_q=chat-1&route_limit=10'
		);

		expect(fetcher).toHaveBeenCalledWith(
			'/api/routes?connector_id=telegram&status=all&q=chat-1&limit=10',
			expect.any(Object)
		);
		expect(data.filters).toEqual({
			connector_id: 'telegram',
			status: 'all',
			query: 'chat-1',
			limit: 10
		});
		expect(data.items).toEqual([
			expect.objectContaining({
				id: 'route-1',
				session_id: 'sess-1',
				connector_id: 'telegram',
				external_id: 'chat-1',
				status: 'inactive',
				status_label: 'Inactive',
				role_label: 'Front',
				deactivation_reason: 'deactivated'
			})
		]);
		expect(data.paging.nextHref).toBe(
			'/channels?tab=settings&route_connector_id=telegram&route_status=all&route_q=chat-1&route_limit=10&route_cursor=cursor-next&route_direction=next'
		);
	});
});
