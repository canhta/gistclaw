import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch): Parameters<typeof load>[0] {
	return { fetch: fetcher } as unknown as Parameters<typeof load>[0];
}

describe('config load', () => {
	it('loads settings from /api/settings', async () => {
		const fetcher = vi.fn<typeof fetch>(
			async () =>
				new Response(
					JSON.stringify({
						machine: {
							storage_root: '/home/user/.gistclaw',
							approval_mode: 'on_request',
							approval_mode_label: 'On Request',
							host_access_mode: 'local',
							host_access_mode_label: 'Local',
							admin_token: 'tok-123',
							per_run_token_budget: '50000',
							daily_cost_cap_usd: '5.00',
							rolling_cost_usd: 0.42,
							rolling_cost_label: '$0.42',
							telegram_token: '',
							active_project_name: 'my-project',
							active_project_path: '/home/user/my-project',
							active_project_summary: '3 agents'
						},
						access: {
							password_configured: true,
							other_active_devices: [],
							blocked_devices: []
						}
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				)
		);

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected config load to return data');
		}

		expect(fetcher).toHaveBeenCalledWith('/api/settings', expect.any(Object));
		expect(result.config.settings?.machine.per_run_token_budget).toBe('50000');
	});

	it('returns null settings when the request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected config load to return fallback data');
		}

		expect(result).toEqual({
			config: {
				settings: null
			}
		});
	});
});
