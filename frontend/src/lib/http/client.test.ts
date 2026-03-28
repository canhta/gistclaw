import { describe, expect, it } from 'vitest';
import { HTTPError, requestJSON } from './client';

describe('requestJSON', () => {
	it('parses successful JSON responses', async () => {
		const fetcher: typeof fetch = async () =>
			new Response(JSON.stringify({ ok: true, value: 'ready' }), {
				status: 200,
				headers: { 'content-type': 'application/json' }
			});

		const result = await requestJSON<{ ok: boolean; value: string }>(fetcher, '/api/bootstrap');

		expect(result).toEqual({ ok: true, value: 'ready' });
	});

	it('throws an HTTPError with the API message when the request fails', async () => {
		const fetcher: typeof fetch = async () =>
			new Response(JSON.stringify({ message: 'Password did not match. Try again.' }), {
				status: 401,
				statusText: 'Unauthorized',
				headers: { 'content-type': 'application/json' }
			});

		await expect(requestJSON(fetcher, '/api/auth/login')).rejects.toEqual(
			new HTTPError(401, 'Password did not match. Try again.')
		);
	});
});
