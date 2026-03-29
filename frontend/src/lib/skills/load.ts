import { requestJSON } from '$lib/http/client';
import type { ExtensionStatusResponse } from '$lib/types/api';

export async function loadExtensionStatus(fetcher: typeof fetch): Promise<ExtensionStatusResponse> {
	return requestJSON<ExtensionStatusResponse>(fetcher, '/api/skills');
}

export function fallbackExtensionStatus(
	notice = 'Skills status could not be loaded. Reload to retry.'
): ExtensionStatusResponse {
	return {
		notice,
		summary: {
			shipped_surfaces: 0,
			configured_surfaces: 0,
			installed_tools: 0,
			ready_credentials: 0,
			missing_credentials: 0
		},
		surfaces: [],
		tools: []
	};
}
