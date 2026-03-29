import { buildChannelStatusData, type ChannelStatusItem } from '$lib/channels/status';
import type { ConversationsResponse, WorkClusterResponse, WorkIndexResponse } from '$lib/types/api';

export interface InstancePresenceLane {
	id: string;
	kind: 'front' | 'specialist';
	agent_id: string;
	objective: string;
	status: string;
	status_label: string;
	status_class: string;
	model_display: string;
	token_summary: string;
	last_activity_short: string;
}

export interface InstancePresenceData {
	summary: {
		front_lane_count: number;
		specialist_lane_count: number;
		live_connector_count: number;
		pending_delivery_count: number;
	};
	lanes: InstancePresenceLane[];
	connectors: ChannelStatusItem[];
	sources: {
		queue_headline: string;
		root_runs: number;
		active_runs: number;
		needs_approval_runs: number;
		session_count: number;
		connector_count: number;
		terminal_deliveries: number;
	};
}

export function buildInstancePresenceData(
	work: WorkIndexResponse | null,
	conversations: ConversationsResponse | null
): InstancePresenceData {
	const connectors = buildChannelStatusData(
		conversations?.runtime_connectors ?? [],
		conversations?.health ?? []
	);
	const lanes = buildPresenceLanes(work?.clusters ?? []);

	return {
		summary: {
			front_lane_count: lanes.filter((lane) => lane.kind === 'front').length,
			specialist_lane_count: lanes.filter((lane) => lane.kind === 'specialist').length,
			live_connector_count: connectors.summary.active_count,
			pending_delivery_count: connectors.summary.pending_count
		},
		lanes,
		connectors: connectors.items,
		sources: {
			queue_headline: work?.queue_strip.headline ?? 'No work queue data',
			root_runs: work?.queue_strip.root_runs ?? 0,
			active_runs: work?.queue_strip.summary.active ?? 0,
			needs_approval_runs: work?.queue_strip.summary.needs_approval ?? 0,
			session_count: conversations?.summary.session_count ?? 0,
			connector_count: conversations?.summary.connector_count ?? connectors.summary.connector_count,
			terminal_deliveries: conversations?.summary.terminal_deliveries ?? 0
		}
	};
}

function buildPresenceLanes(clusters: WorkClusterResponse[]): InstancePresenceLane[] {
	return clusters
		.flatMap((cluster) => [cluster.root, ...(cluster.children ?? [])])
		.filter((run) => isPresenceStatus(run.status))
		.map<InstancePresenceLane>((run) => {
			const kind: InstancePresenceLane['kind'] = run.depth === 0 ? 'front' : 'specialist';

			return {
				id: run.id,
				kind,
				agent_id: run.agent_id,
				objective: run.objective,
				status: run.status,
				status_label: run.status_label,
				status_class: run.status_class,
				model_display: run.model_display,
				token_summary: run.token_summary,
				last_activity_short: run.last_activity_short
			};
		})
		.sort((left, right) => {
			const kindDelta = laneKindRank(left) - laneKindRank(right);
			if (kindDelta !== 0) {
				return kindDelta;
			}

			const statusDelta = laneStatusRank(right.status) - laneStatusRank(left.status);
			if (statusDelta !== 0) {
				return statusDelta;
			}

			return left.agent_id.localeCompare(right.agent_id);
		});
}

function isPresenceStatus(status: string): boolean {
	return status === 'active' || status === 'pending' || status === 'needs_approval';
}

function laneKindRank(lane: InstancePresenceLane): number {
	return lane.kind === 'front' ? 0 : 1;
}

function laneStatusRank(status: string): number {
	switch (status) {
		case 'active':
			return 3;
		case 'needs_approval':
			return 2;
		case 'pending':
			return 1;
		default:
			return 0;
	}
}
