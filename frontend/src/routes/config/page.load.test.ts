import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch): Parameters<typeof load>[0] {
	return { fetch: fetcher } as unknown as Parameters<typeof load>[0];
}

describe('config load', () => {
	it('loads settings, team data, and recent work for config', async () => {
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

			if (input === '/api/work') {
				return new Response(
					JSON.stringify({
						active_project_name: 'my-project',
						active_project_path: '/home/user/my-project',
						queue_strip: {
							headline: '1 active run',
							root_runs: 1,
							worker_runs: 1,
							recovery_runs: 0,
							summary: {
								total: 2,
								pending: 0,
								active: 1,
								needs_approval: 0,
								completed: 1,
								failed: 0,
								interrupted: 0,
								root_status: 'active'
							}
						},
						paging: { has_next: false, has_prev: false },
						clusters: [
							{
								root: {
									id: 'run-root',
									objective: 'Review the repo',
									agent_id: 'assistant',
									status: 'active',
									status_label: 'Active',
									status_class: 'is-active',
									model_display: 'gpt-5.4',
									token_summary: '1K tokens',
									started_at_short: '10:00',
									started_at_exact: '2026-03-29 10:00',
									started_at_iso: '2026-03-29T10:00:00Z',
									last_activity_short: '10:05',
									last_activity_exact: '2026-03-29 10:05',
									last_activity_iso: '2026-03-29T10:05:00Z',
									depth: 0
								},
								children: [],
								child_count: 0,
								child_count_label: '0 child runs',
								blocker_label: '',
								has_children: false
							}
						]
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
		expect(fetcher).toHaveBeenNthCalledWith(3, '/api/work', expect.any(Object));
		expect(result.config.settings?.machine.per_run_token_budget).toBe('50000');
		expect(result.config.team?.team.name).toBe('Repo Task Team');
		expect(result.config.team?.team.front_agent_id).toBe('assistant');
		expect(result.config.work?.clusters[0]?.root.model_display).toBe('gpt-5.4');
	});

	it('returns partial fallback data when one request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			if (input === '/api/settings') {
				throw new Error('boom');
			}

			return new Response(
				JSON.stringify({
					...(input === '/api/team'
						? {
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
							}
						: {
								active_project_name: 'my-project',
								active_project_path: '/home/user/my-project',
								queue_strip: {
									headline: '1 active run',
									root_runs: 1,
									worker_runs: 0,
									recovery_runs: 0,
									summary: {
										total: 1,
										pending: 0,
										active: 1,
										needs_approval: 0,
										completed: 0,
										failed: 0,
										interrupted: 0,
										root_status: 'active'
									}
								},
								paging: { has_next: false, has_prev: false },
								clusters: [
									{
										root: {
											id: 'run-root',
											objective: 'Review the repo',
											agent_id: 'assistant',
											status: 'active',
											status_label: 'Active',
											status_class: 'is-active',
											model_display: 'gpt-5.4-mini',
											token_summary: '600 tokens',
											started_at_short: '10:00',
											started_at_exact: '2026-03-29 10:00',
											started_at_iso: '2026-03-29T10:00:00Z',
											last_activity_short: '10:05',
											last_activity_exact: '2026-03-29 10:05',
											last_activity_iso: '2026-03-29T10:05:00Z',
											depth: 0
										},
										children: [],
										child_count: 0,
										child_count_label: '0 child runs',
										blocker_label: '',
										has_children: false
									}
								]
							})
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
				}),
				work: expect.objectContaining({
					clusters: [
						expect.objectContaining({
							root: expect.objectContaining({
								model_display: 'gpt-5.4-mini'
							})
						})
					]
				})
			}
		});
	});
});
