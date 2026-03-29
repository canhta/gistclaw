import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch): Parameters<typeof load>[0] {
	return { fetch: fetcher } as unknown as Parameters<typeof load>[0];
}

describe('channels load', () => {
	it('loads a merged channels status board from conversations', async () => {
		const fetcher = vi.fn<typeof fetch>(
			async () =>
				new Response(
					JSON.stringify({
						summary: {
							session_count: 2,
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
						paging: { has_next: false, has_prev: false },
						health: [
							{
								connector_id: 'telegram',
								pending_count: 1,
								retrying_count: 0,
								terminal_count: 0,
								state_class: 'is-success'
							}
						],
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
				)
		);

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected channels load to return data');
		}

		expect(fetcher).toHaveBeenCalledWith('/api/conversations', expect.any(Object));
		expect(result.channels.summary).toEqual({
			connector_count: 1,
			active_count: 1,
			pending_count: 1,
			retrying_count: 0,
			terminal_count: 0,
			restart_suggested_count: 0
		});
		expect(result.channels.items).toEqual([
			expect.objectContaining({
				connector_id: 'telegram',
				state_label: 'Active',
				pending_count: 1,
				retrying_count: 0,
				terminal_count: 0
			})
		]);
	});

	it('returns empty channels data when the request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected channels load to return fallback data');
		}

		expect(result).toEqual({
			channels: {
				summary: {
					connector_count: 0,
					active_count: 0,
					pending_count: 0,
					retrying_count: 0,
					terminal_count: 0,
					restart_suggested_count: 0
				},
				items: []
			}
		});
	});
});
