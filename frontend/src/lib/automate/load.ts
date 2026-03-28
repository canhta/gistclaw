import { requestJSON } from '$lib/http/client';
import type { AutomateResponse } from '$lib/types/api';

export function loadAutomate(fetcher: typeof fetch): Promise<AutomateResponse> {
	return requestJSON<AutomateResponse>(fetcher, '/api/automate');
}
