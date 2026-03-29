import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch, search = ''): Parameters<typeof load>[0] {
	return {
		fetch: fetcher,
		url: new URL(`http://localhost/logs${search}`)
	} as unknown as Parameters<typeof load>[0];
}

describe('logs load', () => {
	it('loads the filtered log snapshot for the current tab state', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			expect(String(input)).toBe('/api/logs?q=panic&level=warn&source=web&limit=100');

			return new Response(
				JSON.stringify({
					summary: {
						buffered_entries: 14,
						visible_entries: 2,
						error_entries: 0,
						warning_entries: 2
					},
					filters: {
						query: 'panic',
						level: 'warn',
						source: 'web',
						limit: 100
					},
					sources: ['runtime', 'scheduler', 'web'],
					stream_url: '/api/logs/stream?q=panic&level=warn&source=web&limit=100',
					entries: [
						{
							id: 11,
							source: 'web',
							level: 'warn',
							level_label: 'Warn',
							message: 'panic method=GET path=/api/debug err=boom',
							raw: 'web warn panic method=GET path=/api/debug err=boom',
							created_at_label: '2026-03-29 10:00:00 UTC'
						}
					]
				}),
				{ status: 200, headers: { 'content-type': 'application/json' } }
			);
		});

		const result = await load(
			makeLoadEvent(fetcher, '?tab=filters&q=panic&level=warn&source=web&limit=100')
		);

		if (!result) {
			throw new Error('expected logs load to return data');
		}

		expect(result.logs.summary.buffered_entries).toBe(14);
		expect(result.logs.filters.source).toBe('web');
		expect(result.logs.entries[0].message).toContain('panic');
		expect(result.logs.stream_url).toBe('/api/logs/stream?q=panic&level=warn&source=web&limit=100');
	});

	it('returns an empty fallback when the logs request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected logs load to return fallback data');
		}

		expect(result).toEqual({
			logs: {
				summary: {
					buffered_entries: 0,
					visible_entries: 0,
					error_entries: 0,
					warning_entries: 0
				},
				filters: {
					query: '',
					level: 'all',
					source: 'all',
					limit: 200
				},
				sources: [],
				stream_url: '/api/logs/stream?limit=200',
				entries: []
			}
		});
	});
});
