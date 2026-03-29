import { describe, expect, it } from 'vitest';
import type { TeamConfigResponse } from '$lib/types/api';
import { teamConfigMatches, teamDraftFromConfig } from './team-state';

function makeTeam(overrides: Partial<TeamConfigResponse> = {}): TeamConfigResponse {
	return {
		name: 'Default Team',
		front_agent_id: 'assistant',
		member_count: 2,
		members: [
			{
				id: 'assistant',
				role: 'front assistant',
				soul_file: 'assistant.soul.yaml',
				base_profile: 'operator',
				tool_families: ['delegate', 'repo_read'],
				delegation_kinds: ['write', 'review'],
				can_message: ['patcher', 'reviewer'],
				specialist_summary_visibility: 'full',
				soul_extra: { rank: 1 },
				is_front: true
			},
			{
				id: 'patcher',
				role: 'scoped write specialist',
				soul_file: 'patcher.soul.yaml',
				base_profile: 'write',
				tool_families: ['repo_read', 'repo_write'],
				delegation_kinds: [],
				can_message: ['assistant'],
				specialist_summary_visibility: 'basic',
				soul_extra: {},
				is_front: false
			}
		],
		...overrides
	};
}

describe('teamConfigMatches', () => {
	it('treats equivalent drafts as unchanged', () => {
		const base = makeTeam();
		const draft = makeTeam({
			members: [
				{
					...base.members[0],
					can_message: ['reviewer', 'patcher'],
					delegation_kinds: ['review', 'write'],
					tool_families: ['repo_read', 'delegate']
				},
				{
					...base.members[1],
					can_message: ['assistant'],
					tool_families: ['repo_write', 'repo_read']
				}
			]
		});

		expect(teamConfigMatches(draft, base)).toBe(true);
	});

	it('detects a changed team name or front agent', () => {
		const base = makeTeam();

		expect(teamConfigMatches(makeTeam({ name: 'Renamed Team' }), base)).toBe(false);
		expect(teamConfigMatches(makeTeam({ front_agent_id: 'patcher' }), base)).toBe(false);
	});

	it('clones an imported config into editable draft state', () => {
		const team = makeTeam();
		const draft = teamDraftFromConfig(team);

		expect(draft).toEqual({
			name: team.name,
			front_agent_id: team.front_agent_id,
			members: team.members
		});
		expect(draft.members).not.toBe(team.members);
		expect(draft.members[0].tool_families).not.toBe(team.members[0].tool_families);
		expect(draft.members[0].can_message).not.toBe(team.members[0].can_message);
	});
});
