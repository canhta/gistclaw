import { requestJSON } from '$lib/http/client';
import type { TeamResponse } from '$lib/types/api';

function teamMutationInit(body: unknown): RequestInit {
	return {
		method: 'POST',
		headers: {
			'content-type': 'application/json'
		},
		body: JSON.stringify(body)
	};
}

export function selectTeamProfile(fetcher: typeof fetch, profileID: string): Promise<TeamResponse> {
	return requestJSON<TeamResponse>(
		fetcher,
		'/api/team/select',
		teamMutationInit({ profile_id: profileID })
	);
}

export function createTeamProfile(fetcher: typeof fetch, profileID: string): Promise<TeamResponse> {
	return requestJSON<TeamResponse>(
		fetcher,
		'/api/team/create',
		teamMutationInit({ profile_id: profileID })
	);
}

export function cloneTeamProfile(
	fetcher: typeof fetch,
	sourceProfileID: string,
	profileID: string
): Promise<TeamResponse> {
	return requestJSON<TeamResponse>(
		fetcher,
		'/api/team/clone',
		teamMutationInit({ source_profile_id: sourceProfileID, profile_id: profileID })
	);
}

export function deleteTeamProfile(fetcher: typeof fetch, profileID: string): Promise<TeamResponse> {
	return requestJSON<TeamResponse>(
		fetcher,
		'/api/team/delete',
		teamMutationInit({ profile_id: profileID })
	);
}
