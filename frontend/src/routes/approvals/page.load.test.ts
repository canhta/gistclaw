import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch): Parameters<typeof load>[0] {
	return { fetch: fetcher } as unknown as Parameters<typeof load>[0];
}

describe('approvals load', () => {
	it('loads approvals and paging from recover', async () => {
		const fetcher = vi.fn<typeof fetch>(
			async () =>
				new Response(
					JSON.stringify({
						summary: {
							open_approvals: 1,
							pending_approvals: 1,
							connector_count: 2,
							active_routes: 3,
							terminal_deliveries: 0
						},
						approvals: [
							{
								id: 'appr-1',
								run_id: 'run-abc',
								tool_name: 'bash',
								binding_summary: 'echo hello',
								status: 'pending',
								status_label: 'Pending',
								status_class: 'is-active'
							}
						],
						approval_paging: { has_next: false, has_prev: false },
						repair: {
							connector_count: 0,
							filters: {
								query: '',
								connector_id: '',
								route_status: '',
								delivery_status: '',
								active_limit: 25,
								history_limit: 25,
								delivery_limit: 25
							},
							health: [],
							runtime_connectors: [],
							active_routes: [],
							active_paging: { has_next: false, has_prev: false },
							route_history: [],
							history_paging: { has_next: false, has_prev: false },
							deliveries: [],
							delivery_paging: { has_next: false, has_prev: false }
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
			throw new Error('expected approvals load to return data');
		}

		expect(fetcher).toHaveBeenCalledWith('/api/recover?status=pending', expect.any(Object));
		expect(result.approvals.items).toHaveLength(1);
		expect(result.approvals.openCount).toBe(1);
		expect(result.approvals.summary).toEqual({
			pendingCount: 1,
			connectorCount: 2,
			activeRoutes: 3
		});
	});

	it('returns empty approvals data when the request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected approvals load to return fallback data');
		}

		expect(result).toEqual({
			approvals: {
				items: [],
				paging: { has_next: false, has_prev: false },
				openCount: 0,
				summary: {
					pendingCount: 0,
					connectorCount: 0,
					activeRoutes: 0
				}
			}
		});
	});
});
