import { requestJSON } from '$lib/http/client';
import type { InstancesResponse } from '$lib/types/api';

export async function loadInstances(fetcher: typeof fetch): Promise<InstancesResponse> {
	return requestJSON<InstancesResponse>(fetcher, '/api/instances');
}
