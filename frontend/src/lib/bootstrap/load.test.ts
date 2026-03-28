import { describe, expect, it, vi } from 'vitest';
import { loadAppShell, resolveEntryHref } from './load';

describe('loadAppShell', () => {
	it('returns auth-only state when the browser is signed out', async () => {
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

		const result = await loadAppShell(fetcher);

		expect(fetcher).toHaveBeenCalledTimes(1);
		expect(fetcher).toHaveBeenCalledWith('/api/auth/session', expect.any(Object));
		expect(result.auth.authenticated).toBe(false);
		expect(result.auth.login_reason).toBe('expired');
		expect(resolveEntryHref(result)).toBe('/login');
		expect(result.onboarding).toBeNull();
		expect(result.project).toBeNull();
		expect(result.navigation).toEqual([]);
	});

	it('loads bootstrap navigation, onboarding state, and project context for authenticated sessions', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			if (input === '/api/auth/session') {
				return new Response(
					JSON.stringify({
						authenticated: true,
						password_configured: true,
						setup_required: false,
						device_id: 'device-local'
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			return new Response(
				JSON.stringify({
					auth: {
						authenticated: true,
						password_configured: true,
						setup_required: false,
						device_id: 'device-local'
					},
					onboarding: {
						completed: false,
						entry_href: '/onboarding'
					},
					project: {
						active_id: 'proj-primary',
						active_name: 'starter-project',
						active_path: '/tmp/starter-project'
					},
					navigation: [
						{ id: 'work', label: 'Work', href: '/work' },
						{ id: 'recover', label: 'Recover', href: '/recover' }
					]
				}),
				{
					status: 200,
					headers: { 'content-type': 'application/json' }
				}
			);
		});

		const result = await loadAppShell(fetcher);

		expect(fetcher).toHaveBeenCalledTimes(2);
		expect(fetcher.mock.calls.map(([input]) => input)).toEqual([
			'/api/auth/session',
			'/api/bootstrap'
		]);
		expect(result.onboarding).toEqual({
			completed: false,
			entry_href: '/onboarding'
		});
		expect(resolveEntryHref(result)).toBe('/onboarding');
		expect(result.project).toEqual({
			active_id: 'proj-primary',
			active_name: 'starter-project',
			active_path: '/tmp/starter-project'
		});
		expect(result.navigation).toEqual([
			{ id: 'work', label: 'Work', href: '/work' },
			{ id: 'recover', label: 'Recover', href: '/recover' }
		]);
	});
});
