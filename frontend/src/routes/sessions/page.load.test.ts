import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch, search = ''): Parameters<typeof load>[0] {
	return {
		fetch: fetcher,
		url: new URL(`http://localhost/sessions${search}`)
	} as unknown as Parameters<typeof load>[0];
}

describe('sessions load', () => {
	it('loads sessions, paging, and runtime connectors from conversations', async () => {
		const fetcher = vi.fn<typeof fetch>(
			async () =>
				new Response(
					JSON.stringify({
						summary: {
							session_count: 1,
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
						sessions: [
							{
								id: 'sess-1',
								conversation_id: 'conv-1',
								agent_id: 'front',
								role_label: 'User',
								status_label: 'Active',
								updated_at_label: '2 min ago'
							}
						],
						paging: { has_next: false, has_prev: false },
						health: [],
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

		const result = await load(makeLoadEvent(fetcher, '?status=active'));

		if (!result) {
			throw new Error('expected sessions load to return data');
		}

		expect(fetcher).toHaveBeenCalledWith('/api/conversations?status=active', expect.any(Object));
		expect(result.sessions.items).toHaveLength(1);
		expect(result.sessions.runtimeConnectors).toHaveLength(1);
	});

	it('returns empty sessions data when the request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected sessions load to return fallback data');
		}

		expect(result).toEqual({
			sessions: {
				items: [],
				paging: { has_next: false, has_prev: false },
				runtimeConnectors: []
			}
		});
	});
});
