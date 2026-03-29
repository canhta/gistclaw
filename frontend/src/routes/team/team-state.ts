import type { TeamConfigResponse, TeamMemberResponse } from '$lib/types/api';

function stableValue(value: unknown): unknown {
	if (Array.isArray(value)) {
		return value.map((item) => stableValue(item));
	}
	if (value && typeof value === 'object') {
		return Object.fromEntries(
			Object.entries(value as Record<string, unknown>)
				.sort(([left], [right]) => left.localeCompare(right))
				.map(([key, item]) => [key, stableValue(item)])
		);
	}
	return value;
}

function sorted(values: string[]): string[] {
	return [...values].sort();
}

function memberSignature(member: TeamMemberResponse): Record<string, unknown> {
	return {
		base_profile: member.base_profile,
		can_message: sorted(member.can_message),
		delegation_kinds: sorted(member.delegation_kinds),
		id: member.id,
		is_front: member.is_front,
		role: member.role,
		soul_extra: stableValue(member.soul_extra),
		soul_file: member.soul_file,
		specialist_summary_visibility: member.specialist_summary_visibility,
		tool_families: sorted(member.tool_families)
	};
}

function teamSignature(team: TeamConfigResponse): Record<string, unknown> {
	return {
		front_agent_id: team.front_agent_id,
		member_count: team.member_count,
		members: team.members.map((member) => memberSignature(member)),
		name: team.name
	};
}

export function teamConfigMatches(left: TeamConfigResponse, right: TeamConfigResponse): boolean {
	return JSON.stringify(teamSignature(left)) === JSON.stringify(teamSignature(right));
}

export interface TeamDraftSnapshot {
	name: string;
	front_agent_id: string;
	members: TeamMemberResponse[];
}

export function teamDraftFromConfig(team: TeamConfigResponse): TeamDraftSnapshot {
	return {
		name: team.name,
		front_agent_id: team.front_agent_id,
		members: team.members.map((member) => ({
			...member,
			can_message: [...member.can_message],
			delegation_kinds: [...member.delegation_kinds],
			tool_families: [...member.tool_families]
		}))
	};
}
