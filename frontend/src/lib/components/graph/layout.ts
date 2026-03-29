import type { Edge, Node } from '@xyflow/svelte';
import type {
	WorkGraphEdgeResponse,
	WorkGraphNodeResponse,
	WorkGraphResponse
} from '$lib/types/api';

export interface FlowRunNodeData extends Record<string, unknown> {
	runID: string;
	shortID: string;
	agentID: string;
	objective: string;
	status: string;
	statusLabel: string;
	statusClass: string;
	modelDisplay: string;
	tokenSummary: string;
	timeLabel: string;
	isActivePath: boolean;
	isRoot: boolean;
	childCount: number;
	isSelected: boolean;
}

export type FlowRunNode = Node<FlowRunNodeData, 'run'>;
export type FlowRunEdge = Edge;

export function buildFlowGraph(
	graph: WorkGraphResponse,
	selectedNodeID: string | null = null
): {
	nodes: FlowRunNode[];
	edges: FlowRunEdge[];
} {
	const laneOrder = orderedLanes(graph.nodes);
	const laneDepthCounts = new Map<string, number>();

	const nodes = [...graph.nodes].sort(compareGraphNodes).map((node) => {
		const laneKey = `${node.lane_id || 'lane'}:${node.depth}`;
		const laneOffset = laneDepthCounts.get(laneKey) ?? 0;
		laneDepthCounts.set(laneKey, laneOffset + 1);

		const laneIndex = Math.max(laneOrder.indexOf(node.lane_id || 'lane'), 0);

		return {
			id: node.id,
			type: 'run',
			position: {
				x: 72 + node.depth * 320,
				y: 72 + laneIndex * 220 + laneOffset * 168
			},
			data: {
				runID: node.id,
				shortID: node.short_id,
				agentID: node.agent_id,
				objective: node.objective_preview || node.objective,
				status: node.status,
				statusLabel: node.status_label,
				statusClass: node.status_class,
				modelDisplay: node.model_display,
				tokenSummary: node.token_summary,
				timeLabel: node.time_label,
				isActivePath: node.is_active_path,
				isRoot: node.is_root,
				childCount: node.child_count,
				isSelected: node.id === selectedNodeID
			},
			draggable: false,
			selectable: true
		} satisfies FlowRunNode;
	});

	const edges = graph.edges.map((edge) => buildFlowEdge(edge, graph.active_path));

	return { nodes, edges };
}

function orderedLanes(nodes: WorkGraphNodeResponse[]): string[] {
	const seen = new Set<string>();
	const lanes: string[] = [];

	for (const node of [...nodes].sort(compareGraphNodes)) {
		const laneID = node.lane_id || 'lane';
		if (seen.has(laneID)) {
			continue;
		}
		seen.add(laneID);
		lanes.push(laneID);
	}

	return lanes;
}

function compareGraphNodes(left: WorkGraphNodeResponse, right: WorkGraphNodeResponse): number {
	if (left.depth !== right.depth) {
		return left.depth - right.depth;
	}
	if (left.lane_id !== right.lane_id) {
		return left.lane_id.localeCompare(right.lane_id);
	}
	if (left.is_root !== right.is_root) {
		return left.is_root ? -1 : 1;
	}
	return left.id.localeCompare(right.id);
}

function buildFlowEdge(edge: WorkGraphEdgeResponse, activePath: string[]): FlowRunEdge {
	return {
		id: edge.id,
		source: edge.from,
		target: edge.to,
		label: edge.label,
		type: 'smoothstep',
		animated: edge.kind === 'delegates' && activePath.includes(edge.to),
		selectable: false
	};
}
