import { requestJSON } from '$lib/http/client';
import type { SettingsResponse } from '$lib/types/api';

export function loadSettings(fetcher: typeof fetch): Promise<SettingsResponse> {
	return requestJSON<SettingsResponse>(fetcher, '/api/settings');
}
