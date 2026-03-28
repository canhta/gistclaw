import { requestJSON } from '$lib/http/client';
import type { WorkDetailResponse, WorkIndexResponse } from '$lib/types/api';

export async function loadWorkIndex(fetcher: typeof fetch): Promise<WorkIndexResponse> {
	return requestJSON<WorkIndexResponse>(fetcher, '/api/work');
}

export async function loadWorkDetail(
	fetcher: typeof fetch,
	runID: string
): Promise<WorkDetailResponse> {
	return requestJSON<WorkDetailResponse>(fetcher, `/api/work/${runID}`);
}
