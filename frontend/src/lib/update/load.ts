import { requestJSON } from '$lib/http/client';
import type { UpdateStatusResponse } from '$lib/types/api';

export async function loadUpdateStatus(fetcher: typeof fetch): Promise<UpdateStatusResponse> {
	return requestJSON<UpdateStatusResponse>(fetcher, '/api/update');
}
