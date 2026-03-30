import { isRedirect } from '@sveltejs/kit';
import { describe, expect, it, vi } from 'vitest';
import { load } from './+layout';

function makeLayoutEvent(fetcher: typeof fetch, path: string): Parameters<typeof load>[0] {
	return {
		fetch: fetcher,
		url: new URL(`http://localhost${path}`)
	} as unknown as Parameters<typeof load>[0];
}

describe('root layout load', () => {
	it('redirects signed-out browsers away from protected routes before page data loads', async () => {
		const fetcher = vi.fn<typeof fetch>(
			async () =>
				new Response(
					JSON.stringify({
						authenticated: false,
						password_configured: true,
						setup_required: false
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				)
		);

		let result: unknown;
		try {
			result = await load(makeLayoutEvent(fetcher, '/chat?tab=graph'));
		} catch (error) {
			result = error;
		}

		expect(fetcher).toHaveBeenCalledTimes(1);
		expect(fetcher).toHaveBeenCalledWith('/api/auth/session', expect.any(Object));
		expect(isRedirect(result)).toBe(true);
		if (!isRedirect(result)) {
			throw new Error('expected redirect result');
		}
		expect(result.status).toBe(307);
		expect(result.location).toBe('/login?next=%2Fchat%3Ftab%3Dgraph');
	});

	it('keeps the login route available for signed-out browsers', async () => {
		const fetcher = vi.fn<typeof fetch>(
			async () =>
				new Response(
					JSON.stringify({
						authenticated: false,
						password_configured: true,
						setup_required: false,
						login_reason: 'expired'
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				)
		);

		const result = await load(makeLayoutEvent(fetcher, '/login?reason=expired'));

		if (!result) {
			throw new Error('expected layout load result');
		}

		expect(result.auth.authenticated).toBe(false);
		expect(result.currentPath).toBe('/login');
		expect(result.currentSearch).toBe('?reason=expired');
	});
});
