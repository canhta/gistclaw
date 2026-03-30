import { requestJSON } from '$lib/http/client';
import type { DebugEventsResponse, DebugRPCStatusResponse } from '$lib/types/api';

interface DeliveryHealthRaw {
	Connectors?: Array<{
		ConnectorID: string;
		PendingCount: number;
		RetryingCount: number;
		TerminalCount: number;
	}>;
	RuntimeConnectors?: Array<{
		ConnectorID: string;
		State: string;
		Summary: string;
		CheckedAt?: string;
		RestartSuggested?: boolean;
	}>;
}

export interface DebugDeliveryHealthResponse {
	connectors: Array<{
		connector_id: string;
		pending_count: number;
		retrying_count: number;
		terminal_count: number;
	}>;
	runtime_connectors: Array<{
		connector_id: string;
		state: string;
		summary: string;
		checked_at?: string;
		restart_suggested: boolean;
	}>;
}

export async function loadDeliveryHealth(
	fetcher: typeof fetch
): Promise<DebugDeliveryHealthResponse> {
	const raw = await requestJSON<DeliveryHealthRaw>(fetcher, '/api/deliveries/health');

	return {
		connectors: (raw.Connectors ?? []).map((entry) => ({
			connector_id: entry.ConnectorID,
			pending_count: entry.PendingCount,
			retrying_count: entry.RetryingCount,
			terminal_count: entry.TerminalCount
		})),
		runtime_connectors: (raw.RuntimeConnectors ?? []).map((entry) => ({
			connector_id: entry.ConnectorID,
			state: entry.State,
			summary: entry.Summary,
			checked_at: entry.CheckedAt,
			restart_suggested: Boolean(entry.RestartSuggested)
		}))
	};
}

export async function loadDebugRPC(
	fetcher: typeof fetch,
	probe?: string | null
): Promise<DebugRPCStatusResponse> {
	const query = new URLSearchParams();
	if (probe && probe.trim() !== '') {
		query.set('probe', probe.trim());
	}

	const suffix = query.size > 0 ? `?${query.toString()}` : '';
	return requestJSON<DebugRPCStatusResponse>(fetcher, `/api/debug/rpc${suffix}`);
}

export async function loadDebugEvents(
	fetcher: typeof fetch,
	runID?: string | null
): Promise<DebugEventsResponse> {
	const query = new URLSearchParams();
	if (runID && runID.trim() !== '') {
		query.set('run_id', runID.trim());
	}

	const suffix = query.size > 0 ? `?${query.toString()}` : '';
	return requestJSON<DebugEventsResponse>(fetcher, `/api/debug/events${suffix}`);
}
