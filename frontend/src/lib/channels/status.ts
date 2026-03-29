import type { RecoverDeliveryHealthResponse, RecoverRuntimeHealthResponse } from '$lib/types/api';

export interface ChannelStatusItem extends RecoverRuntimeHealthResponse {
	pending_count: number;
	retrying_count: number;
	terminal_count: number;
}

export interface ChannelStatusSummary {
	connector_count: number;
	active_count: number;
	pending_count: number;
	retrying_count: number;
	terminal_count: number;
	restart_suggested_count: number;
}

export interface ChannelStatusData {
	summary: ChannelStatusSummary;
	items: ChannelStatusItem[];
}

export function buildChannelStatusData(
	connectors: RecoverRuntimeHealthResponse[],
	health: RecoverDeliveryHealthResponse[]
): ChannelStatusData {
	const connectorMap = new Map(connectors.map((entry) => [entry.connector_id, entry] as const));
	const healthMap = new Map(health.map((entry) => [entry.connector_id, entry] as const));
	const connectorIDs = new Set([...connectorMap.keys(), ...healthMap.keys()]);

	const items = [...connectorIDs]
		.map((connectorID) => {
			const runtime = connectorMap.get(connectorID);
			const queue = healthMap.get(connectorID);

			return {
				connector_id: connectorID,
				state: runtime?.state ?? 'unknown',
				state_label: runtime?.state_label ?? 'Unknown',
				state_class: runtime?.state_class ?? queue?.state_class ?? 'is-muted',
				summary: runtime?.summary ?? 'No runtime snapshot yet.',
				checked_at_label: runtime?.checked_at_label,
				restart_suggested: runtime?.restart_suggested ?? false,
				pending_count: queue?.pending_count ?? 0,
				retrying_count: queue?.retrying_count ?? 0,
				terminal_count: queue?.terminal_count ?? 0
			};
		})
		.sort((left, right) => {
			const issueDelta = connectorIssueRank(right) - connectorIssueRank(left);
			if (issueDelta !== 0) {
				return issueDelta;
			}

			const activityDelta = Number(isActiveConnector(right)) - Number(isActiveConnector(left));
			if (activityDelta !== 0) {
				return activityDelta;
			}

			return left.connector_id.localeCompare(right.connector_id);
		});

	return {
		summary: {
			connector_count: items.length,
			active_count: items.filter(isActiveConnector).length,
			pending_count: items.reduce((sum, item) => sum + item.pending_count, 0),
			retrying_count: items.reduce((sum, item) => sum + item.retrying_count, 0),
			terminal_count: items.reduce((sum, item) => sum + item.terminal_count, 0),
			restart_suggested_count: items.filter((item) => item.restart_suggested).length
		},
		items
	};
}

function isActiveConnector(item: ChannelStatusItem): boolean {
	return (
		item.state === 'active' || item.state_class === 'is-success' || item.state_class === 'is-active'
	);
}

function connectorIssueRank(item: ChannelStatusItem): number {
	let rank = 0;

	if (item.restart_suggested) {
		rank += 8;
	}
	if (item.terminal_count > 0) {
		rank += 4;
	}
	if (item.retrying_count > 0) {
		rank += 2;
	}
	if (item.pending_count > 0) {
		rank += 1;
	}
	if (item.state_class === 'is-error') {
		rank += 4;
	}

	return rank;
}
