import type { WorkDetailResponse, WorkNodeDetailResponse } from '$lib/types/api';
import { loadWorkDetail, loadWorkNodeDetail } from './load';

const graphRefreshEventKinds = new Set([
	'approval_requested',
	'run_completed',
	'run_failed',
	'run_interrupted',
	'run_resumed',
	'run_started',
	'run_updated',
	'tool_call_recorded',
	'tool_completed',
	'tool_failed',
	'tool_log_recorded',
	'tool_started',
	'turn_completed'
]);

export interface LiveWorkSurface {
	detail: WorkDetailResponse;
	nodeDetail: WorkNodeDetailResponse | null;
	inspectorNodeID: string | null;
}

export function shouldRefreshWorkSurface(kind: string): boolean {
	return graphRefreshEventKinds.has(kind);
}

export async function loadLiveWorkSurface(
	fetcher: typeof fetch,
	runID: string,
	requestedNodeID = ''
): Promise<LiveWorkSurface> {
	const detail = await loadWorkDetail(fetcher, runID);
	const seedNodeID = detail.inspector_seed?.id?.trim() ?? '';
	const requestedID = requestedNodeID.trim();
	const candidateNodeIDs = Array.from(
		new Set([requestedID, seedNodeID].filter((value) => value !== ''))
	);

	let nodeDetail: WorkNodeDetailResponse | null = null;
	let inspectorNodeID: string | null = seedNodeID || null;

	for (const nodeID of candidateNodeIDs) {
		try {
			nodeDetail = await loadWorkNodeDetail(fetcher, runID, nodeID);
			inspectorNodeID = nodeID;
			break;
		} catch {
			// Fall back to the inspector seed or leave the inspector empty.
		}
	}

	return { detail, nodeDetail, inspectorNodeID };
}
