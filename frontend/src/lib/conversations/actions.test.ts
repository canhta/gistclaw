import { describe, expect, it, vi } from 'vitest';
import { createRoute, deactivateRoute, retryConversationDelivery } from './actions';

describe('conversation action helpers', () => {
	it('posts delivery retries to the session delivery retry endpoint', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			return new Response(JSON.stringify({ delivery: { id: 'delivery/1', status: 'queued' } }), {
				status: 200,
				headers: { 'content-type': 'application/json' }
			});
		});

		await retryConversationDelivery(fetcher, 'sess-1', 'delivery/1');

		expect(fetcher).toHaveBeenCalledWith(
			'/api/conversations/sess-1/deliveries/delivery%2F1/retry',
			{
				method: 'POST',
				headers: {
					accept: 'application/json'
				}
			}
		);
	});

	it('posts route deactivation to the route deactivate endpoint', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			return new Response(JSON.stringify({ route: { id: 'route-1', status: 'inactive' } }), {
				status: 200,
				headers: { 'content-type': 'application/json' }
			});
		});

		await deactivateRoute(fetcher, 'route-1');

		expect(fetcher).toHaveBeenCalledWith('/api/routes/route-1/deactivate', {
			method: 'POST',
			headers: {
				accept: 'application/json'
			}
		});
	});

	it('posts route binding to the routes endpoint', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			return new Response(JSON.stringify({ route: { id: 'route-1', status: 'active' } }), {
				status: 200,
				headers: { 'content-type': 'application/json' }
			});
		});

		await createRoute(fetcher, {
			sessionID: 'sess-1',
			connectorID: 'telegram',
			externalID: 'chat-1',
			threadID: 'thread-1',
			accountID: 'acct-1'
		});

		expect(fetcher).toHaveBeenCalledWith('/api/routes', {
			method: 'POST',
			headers: {
				accept: 'application/json',
				'content-type': 'application/json'
			},
			body: JSON.stringify({
				session_id: 'sess-1',
				connector_id: 'telegram',
				external_id: 'chat-1',
				thread_id: 'thread-1',
				account_id: 'acct-1'
			})
		});
	});
});
