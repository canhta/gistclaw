import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch): Parameters<typeof load>[0] {
	return {
		fetch: fetcher,
		url: new URL('http://localhost/nodes')
	} as unknown as Parameters<typeof load>[0];
}

describe('nodes load', () => {
	it('loads the node inventory snapshot', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			expect(String(input)).toBe('/api/nodes');

			return new Response(
				JSON.stringify({
					summary: {
						connectors: 2,
						healthy_connectors: 1,
						run_nodes: 2,
						approval_nodes: 1,
						capabilities: 3
					},
					connectors: [
						{
							id: 'telegram',
							aliases: ['tg'],
							exposure: 'remote',
							state: 'healthy',
							state_label: 'healthy',
							summary: 'polling',
							checked_at_label: '2026-03-29 09:30:00 UTC',
							restart_suggested: false
						}
					],
					runs: [
						{
							id: 'run-root',
							short_id: 'run-root',
							parent_run_id: '',
							kind: 'root',
							agent_id: 'assistant',
							status: 'active',
							status_label: 'active',
							objective_preview: 'Review the repo layout',
							started_at_label: '2026-03-29 09:30:00 UTC',
							updated_at_label: '2026-03-29 09:31:00 UTC'
						}
					],
					capabilities: [
						{
							name: 'connector_send',
							family: 'connector',
							description: 'Send a direct message through a configured connector.'
						}
					]
				}),
				{ status: 200, headers: { 'content-type': 'application/json' } }
			);
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected nodes load to return data');
		}

		expect(result.nodes.summary.connectors).toBe(2);
		expect(result.nodes.connectors[0].id).toBe('telegram');
		expect(result.nodes.capabilities[0].name).toBe('connector_send');
	});

	it('returns a safe fallback when the nodes request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected nodes load to return fallback data');
		}

		expect(result.nodes.notice).toBe('Node inventory could not be loaded. Reload to retry.');
		expect(result.nodes.summary.connectors).toBe(0);
		expect(result.nodes.connectors).toEqual([]);
		expect(result.nodes.runs).toEqual([]);
		expect(result.nodes.capabilities).toEqual([]);
	});
});
