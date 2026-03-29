import { requestJSON } from '$lib/http/client';
import type { DebugEventsResponse, DebugRPCStatusResponse } from '$lib/types/api';

const debugRPCProbeCatalog = [
	{
		name: 'status',
		label: 'Status',
		description: 'Inspect active runs, approvals, and storage health.'
	},
	{
		name: 'connector_health',
		label: 'Connector health',
		description: 'Inspect configured connector health snapshots.'
	},
	{
		name: 'active_project',
		label: 'Active project',
		description: 'Inspect the current project scope and workspace path.'
	},
	{
		name: 'schedule_status',
		label: 'Scheduler',
		description: 'Inspect schedule counters and the next scheduler wake time.'
	}
] as const;

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

export function fallbackDebugRPCStatus(
	notice = 'RPC probes could not be loaded. Reload to retry.',
	probe?: string | null
): DebugRPCStatusResponse {
	const selected =
		probe && debugRPCProbeCatalog.some((candidate) => candidate.name === probe.trim())
			? probe.trim()
			: 'status';
	const selectedProbe =
		debugRPCProbeCatalog.find((candidate) => candidate.name === selected) ??
		debugRPCProbeCatalog[0];

	return {
		notice,
		summary: {
			probe_count: debugRPCProbeCatalog.length,
			read_only: true,
			default_probe: 'status',
			selected_probe: selectedProbe.name
		},
		probes: [...debugRPCProbeCatalog],
		result: {
			probe: selectedProbe.name,
			label: selectedProbe.label,
			summary: notice,
			executed_at: '',
			executed_at_label: 'Unavailable',
			data: {}
		}
	};
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
