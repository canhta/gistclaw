import { describe, expect, it, vi } from 'vitest';
import { deactivateRoute, retryConversationDelivery } from './actions';

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
});
