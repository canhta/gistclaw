import { requestJSON } from '$lib/http/client';
import type {
	WorkDetailResponse,
	WorkGraphResponse,
	WorkIndexResponse,
	WorkNodeDetailResponse
} from '$lib/types/api';

export async function loadWorkIndex(
	fetcher: typeof fetch,
	search = ''
): Promise<WorkIndexResponse> {
	const suffix = search.trim() === '' ? '' : `?${search}`;
	return requestJSON<WorkIndexResponse>(fetcher, `/api/work${suffix}`);
}

export async function loadWorkDetail(
	fetcher: typeof fetch,
	runID: string
): Promise<WorkDetailResponse> {
	return requestJSON<WorkDetailResponse>(fetcher, `/api/work/${runID}`);
}

export async function loadWorkGraph(
	fetcher: typeof fetch,
	runID: string
): Promise<WorkGraphResponse> {
	return requestJSON<WorkGraphResponse>(fetcher, `/api/work/${runID}/graph`);
}

export async function loadWorkNodeDetail(
	fetcher: typeof fetch,
	runID: string,
	nodeID: string
): Promise<WorkNodeDetailResponse> {
	return requestJSON<WorkNodeDetailResponse>(
		fetcher,
		`/api/work/${runID}/nodes/${encodeURIComponent(nodeID)}`
	);
}
