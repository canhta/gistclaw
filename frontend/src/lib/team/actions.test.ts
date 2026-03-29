import { describe, expect, it, vi } from 'vitest';
import {
	cloneTeamProfile,
	createTeamProfile,
	deleteTeamProfile,
	selectTeamProfile
} from './actions';

const teamResponse = {
	notice: 'ok',
	active_profile: {
		id: 'default',
		label: 'default',
		active: true,
		save_path: '/tmp/default/team.yaml'
	},
	profiles: [
		{
			id: 'default',
			label: 'default',
			active: true,
			save_path: '/tmp/default/team.yaml'
		}
	],
	team: {
		name: 'Repo Task Team',
		front_agent_id: 'assistant',
		member_count: 1,
		members: []
	}
};

describe('team action helpers', () => {
	it('posts profile selection to the team select endpoint', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			return new Response(JSON.stringify(teamResponse), {
				status: 200,
				headers: { 'content-type': 'application/json' }
			});
		});

		await selectTeamProfile(fetcher, 'safe');

		expect(fetcher).toHaveBeenCalledWith('/api/team/select', {
			method: 'POST',
			headers: {
				accept: 'application/json',
				'content-type': 'application/json'
			},
			body: JSON.stringify({ profile_id: 'safe' })
		});
	});

	it('posts profile creation to the team create endpoint', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			return new Response(JSON.stringify(teamResponse), {
				status: 201,
				headers: { 'content-type': 'application/json' }
			});
		});

		await createTeamProfile(fetcher, 'ops');

		expect(fetcher).toHaveBeenCalledWith('/api/team/create', {
			method: 'POST',
			headers: {
				accept: 'application/json',
				'content-type': 'application/json'
			},
			body: JSON.stringify({ profile_id: 'ops' })
		});
	});

	it('posts profile cloning to the team clone endpoint', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			return new Response(JSON.stringify(teamResponse), {
				status: 201,
				headers: { 'content-type': 'application/json' }
			});
		});

		await cloneTeamProfile(fetcher, 'default', 'ops');

		expect(fetcher).toHaveBeenCalledWith('/api/team/clone', {
			method: 'POST',
			headers: {
				accept: 'application/json',
				'content-type': 'application/json'
			},
			body: JSON.stringify({ source_profile_id: 'default', profile_id: 'ops' })
		});
	});

	it('posts profile deletion to the team delete endpoint', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			return new Response(JSON.stringify(teamResponse), {
				status: 200,
				headers: { 'content-type': 'application/json' }
			});
		});

		await deleteTeamProfile(fetcher, 'ops');

		expect(fetcher).toHaveBeenCalledWith('/api/team/delete', {
			method: 'POST',
			headers: {
				accept: 'application/json',
				'content-type': 'application/json'
			},
			body: JSON.stringify({ profile_id: 'ops' })
		});
	});
});
