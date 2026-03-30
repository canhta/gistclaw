import { requestJSON } from '$lib/http/client';
import type { ExtensionStatusResponse } from '$lib/types/api';

export async function loadExtensionStatus(fetcher: typeof fetch): Promise<ExtensionStatusResponse> {
	return requestJSON<ExtensionStatusResponse>(fetcher, '/api/skills');
}
