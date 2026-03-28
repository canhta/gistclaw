import { requestJSON } from '$lib/http/client';
import type { TeamResponse } from '$lib/types/api';

export function loadTeam(fetcher: typeof fetch): Promise<TeamResponse> {
	return requestJSON<TeamResponse>(fetcher, '/api/team');
}
