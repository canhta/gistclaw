import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch): Parameters<typeof load>[0] {
	return { fetch: fetcher } as unknown as Parameters<typeof load>[0];
}

describe('instances load', () => {
	it('loads the dedicated instances inventory feed', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			if (input === '/api/instances') {
				return new Response(
					JSON.stringify({
						summary: {
							front_lane_count: 1,
							specialist_lane_count: 1,
							live_connector_count: 1,
							pending_delivery_count: 2
						},
						lanes: [
							{
								id: 'run-root',
								kind: 'front',
								agent_id: 'assistant',
								objective: 'Review the repo',
								status: 'active',
								status_label: 'Active',
								status_class: 'is-active',
								model_display: 'gpt-5.4',
								token_summary: '1K tokens',
								last_activity_short: '10:05'
							}
						],
						connectors: [
							{
								connector_id: 'telegram',
								state: 'healthy',
								state_label: 'Healthy',
								state_class: 'is-active',
								summary: 'Presence beacons healthy',
								checked_at_label: '1 min ago',
								restart_suggested: false,
								pending_count: 2,
								retrying_count: 0,
								terminal_count: 0
							}
						],
						sources: {
							queue_headline: '1 active run',
							root_runs: 1,
							active_runs: 1,
							needs_approval_runs: 1,
							session_count: 2,
							connector_count: 1,
							terminal_deliveries: 0
						}
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			throw new Error(`unexpected request: ${String(input)}`);
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected instances load to return data');
		}

		expect(fetcher).toHaveBeenCalledTimes(1);
		expect(fetcher).toHaveBeenNthCalledWith(1, '/api/instances', expect.any(Object));
		expect(result.instances.summary).toEqual({
			front_lane_count: 1,
			specialist_lane_count: 1,
			live_connector_count: 1,
			pending_delivery_count: 2
		});
		expect(result.instances.lanes[0]?.agent_id).toBe('assistant');
	});

	it('returns fallback inventory data when the instances feed fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected instances load to return fallback data');
		}

		expect(result.instances.summary).toEqual({
			front_lane_count: 0,
			specialist_lane_count: 0,
			live_connector_count: 0,
			pending_delivery_count: 0
		});
		expect(result.instances.lanes).toEqual([]);
		expect(result.instances.connectors).toEqual([]);
	});
});
