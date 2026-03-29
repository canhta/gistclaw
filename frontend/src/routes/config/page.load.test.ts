import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch): Parameters<typeof load>[0] {
	return { fetch: fetcher } as unknown as Parameters<typeof load>[0];
}

describe('config load', () => {
	it('loads settings and team data for config', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			if (input === '/api/settings') {
				return new Response(
					JSON.stringify({
						machine: {
							storage_root: '/home/user/.gistclaw',
							approval_mode: 'on_request',
							approval_mode_label: 'On Request',
							host_access_mode: 'local',
							host_access_mode_label: 'Local',
							admin_token: 'tok-123',
							per_run_token_budget: '50000',
							daily_cost_cap_usd: '5.00',
							rolling_cost_usd: 0.42,
							rolling_cost_label: '$0.42',
							telegram_token: '',
							active_project_name: 'my-project',
							active_project_path: '/home/user/my-project',
							active_project_summary: '3 agents'
						},
						access: {
							password_configured: true,
							other_active_devices: [],
							blocked_devices: []
						}
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			if (input === '/api/team') {
				return new Response(
					JSON.stringify({
						notice: 'Loaded from team file',
						active_profile: {
							id: 'default',
							label: 'default',
							active: true,
							save_path: '/home/user/.gistclaw/profiles/default.json5'
						},
						profiles: [
							{
								id: 'default',
								label: 'default',
								active: true,
								save_path: '/home/user/.gistclaw/profiles/default.json5'
							}
						],
						team: {
							name: 'Repo Task Team',
							front_agent_id: 'assistant',
							member_count: 2,
							members: [
								{
									id: 'assistant',
									role: 'front assistant',
									soul_file: 'teams/assistant.md',
									base_profile: 'default',
									tool_families: ['repo_read', 'web_fetch'],
									delegation_kinds: ['reviewer', 'patcher'],
									can_message: ['reviewer', 'patcher'],
									specialist_summary_visibility: 'full',
									soul_extra: {},
									is_front: true
								},
								{
									id: 'reviewer',
									role: 'diff reviewer',
									soul_file: 'teams/reviewer.md',
									base_profile: 'default',
									tool_families: ['repo_read'],
									delegation_kinds: [],
									can_message: [],
									specialist_summary_visibility: 'summary',
									soul_extra: {},
									is_front: false
								}
							]
						}
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			throw new Error(`unexpected request: ${String(input)}`);
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected config load to return data');
		}

		expect(fetcher).toHaveBeenNthCalledWith(1, '/api/settings', expect.any(Object));
		expect(fetcher).toHaveBeenNthCalledWith(2, '/api/team', expect.any(Object));
		expect(result.config.settings?.machine.per_run_token_budget).toBe('50000');
		expect(result.config.team?.team.name).toBe('Repo Task Team');
		expect(result.config.team?.team.front_agent_id).toBe('assistant');
	});

	it('returns partial fallback data when one request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			if (input === '/api/settings') {
				throw new Error('boom');
			}

			return new Response(
				JSON.stringify({
					active_profile: {
						id: 'default',
						label: 'default',
						active: true,
						save_path: '/home/user/.gistclaw/profiles/default.json5'
					},
					profiles: [
						{
							id: 'default',
							label: 'default',
							active: true,
							save_path: '/home/user/.gistclaw/profiles/default.json5'
						}
					],
					team: {
						name: 'Repo Task Team',
						front_agent_id: 'assistant',
						member_count: 1,
						members: [
							{
								id: 'assistant',
								role: 'front assistant',
								soul_file: 'teams/assistant.md',
								base_profile: 'default',
								tool_families: ['repo_read'],
								delegation_kinds: ['reviewer'],
								can_message: ['reviewer'],
								specialist_summary_visibility: 'full',
								soul_extra: {},
								is_front: true
							}
						]
					}
				}),
				{
					status: 200,
					headers: { 'content-type': 'application/json' }
				}
			);
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected config load to return fallback data');
		}

		expect(result).toEqual({
			config: {
				settings: null,
				team: expect.objectContaining({
					team: expect.objectContaining({
						name: 'Repo Task Team'
					})
				})
			}
		});
	});
});
