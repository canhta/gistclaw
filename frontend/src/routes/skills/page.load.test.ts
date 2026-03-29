import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch): Parameters<typeof load>[0] {
	return {
		fetch: fetcher,
		url: new URL('http://localhost/skills')
	} as unknown as Parameters<typeof load>[0];
}

describe('skills load', () => {
	it('loads the extension status snapshot', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			expect(String(input)).toBe('/api/skills');

			return new Response(
				JSON.stringify({
					summary: {
						shipped_surfaces: 6,
						configured_surfaces: 4,
						installed_tools: 18,
						ready_credentials: 4,
						missing_credentials: 1
					},
					surfaces: [
						{
							id: 'anthropic',
							name: 'Anthropic',
							kind: 'provider',
							configured: true,
							active: true,
							credential_state: 'ready',
							credential_state_label: 'ready',
							summary: 'Primary provider is configured.',
							detail: 'cheap claude-3-haiku · strong claude-sonnet'
						}
					],
					tools: [
						{
							name: 'connector_send',
							family: 'connector',
							risk: 'medium',
							approval: 'required',
							side_effect: 'connector_send',
							description: 'Send a direct message through a registered connector target.'
						}
					]
				}),
				{ status: 200, headers: { 'content-type': 'application/json' } }
			);
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected skills load to return data');
		}

		expect(result.skills.summary.shipped_surfaces).toBe(6);
		expect(result.skills.surfaces[0].id).toBe('anthropic');
		expect(result.skills.tools[0].name).toBe('connector_send');
	});

	it('returns a safe fallback when the skills request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected skills load to return fallback data');
		}

		expect(result.skills.summary.shipped_surfaces).toBe(0);
		expect(result.skills.surfaces).toEqual([]);
		expect(result.skills.tools).toEqual([]);
	});
});
