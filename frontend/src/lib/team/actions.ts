import { requestJSON } from '$lib/http/client';
import type { TeamConfigResponse, TeamResponse } from '$lib/types/api';

function teamMutationInit(body: unknown): RequestInit {
	return {
		method: 'POST',
		headers: {
			'content-type': 'application/json'
		},
		body: JSON.stringify(body)
	};
}

function teamSaveBody(team: TeamConfigResponse): { team: Record<string, unknown> } {
	return {
		team: {
			name: team.name,
			front_agent_id: team.front_agent_id,
			members: team.members.map((member) => ({
				id: member.id,
				role: member.role,
				soul_file: member.soul_file,
				base_profile: member.base_profile,
				tool_families: member.tool_families,
				delegation_kinds: member.delegation_kinds,
				can_message: member.can_message,
				specialist_summary_visibility: member.specialist_summary_visibility,
				soul_extra: member.soul_extra
			}))
		}
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

export function importTeamYAML(fetcher: typeof fetch, yaml: string): Promise<TeamResponse> {
	return requestJSON<TeamResponse>(fetcher, '/api/team/import', teamMutationInit({ yaml }));
}

export function saveTeamConfig(
	fetcher: typeof fetch,
	team: TeamConfigResponse
): Promise<TeamResponse> {
	return requestJSON<TeamResponse>(fetcher, '/api/team/save', teamMutationInit(teamSaveBody(team)));
}
