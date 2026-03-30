import { requestJSON } from '$lib/http/client';
import type { InstancesResponse } from '$lib/types/api';

export async function loadInstances(fetcher: typeof fetch): Promise<InstancesResponse> {
	return requestJSON<InstancesResponse>(fetcher, '/api/instances');
}

export function fallbackInstances(): InstancesResponse {
	return {
		summary: {
			front_lane_count: 0,
			specialist_lane_count: 0,
			live_connector_count: 0,
			pending_delivery_count: 0
		},
		lanes: [],
		connectors: [],
		sources: {
			queue_headline: 'No work queue data',
			root_runs: 0,
			active_runs: 0,
			needs_approval_runs: 0,
			session_count: 0,
			connector_count: 0,
			terminal_deliveries: 0
		}
	};
}
