import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import TeamPage from './+page.svelte';

describe('Team page', () => {
	it('renders the active setup, profiles, and member topology', () => {
		const { body } = render(TeamPage, {
			props: {
				data: {
					auth: {
						authenticated: true,
						password_configured: true,
						setup_required: false
					},
					project: {
						active_id: 'proj-primary',
						active_name: 'starter-project',
						active_path: '/tmp/starter-project'
					},
					navigation: [
						{ id: 'work', label: 'Work', href: '/work' },
						{ id: 'team', label: 'Team', href: '/team' }
					],
					currentPath: '/team',
					currentSearch: '',
					team: {
						notice: 'Profile review created and selected.',
						active_profile: {
							id: 'review',
							label: 'review',
							active: true,
							save_path: '/tmp/storage/projects/proj-primary/teams/review/team.yaml'
						},
						profiles: [
							{ id: 'review', label: 'review', active: true },
							{ id: 'default', label: 'default', active: false }
						],
						team: {
							name: 'Review Crew',
							front_agent_id: 'reviewer',
							member_count: 3,
							members: [
								{
									id: 'assistant',
									role: 'front assistant',
									soul_file: 'assistant.soul.yaml',
									base_profile: 'operator',
									tool_families: ['repo_read', 'runtime_capability', 'delegate'],
									delegation_kinds: ['write', 'review'],
									can_message: ['patcher', 'reviewer'],
									specialist_summary_visibility: 'full',
									soul_extra: {},
									is_front: false
								},
								{
									id: 'patcher',
									role: 'scoped write specialist',
									soul_file: 'patcher.soul.yaml',
									base_profile: 'write',
									tool_families: ['repo_read', 'repo_write'],
									delegation_kinds: [],
									can_message: ['assistant', 'reviewer'],
									specialist_summary_visibility: 'basic',
									soul_extra: {},
									is_front: false
								},
								{
									id: 'reviewer',
									role: 'diff reviewer',
									soul_file: 'reviewer.soul.yaml',
									base_profile: 'review',
									tool_families: ['repo_read', 'diff_review'],
									delegation_kinds: [],
									can_message: ['assistant', 'patcher'],
									specialist_summary_visibility: 'basic',
									soul_extra: {},
									is_front: true
								}
							]
						}
					}
				}
			}
		});

		expect(body).toContain('Review Crew');
		expect(body).toContain('Profile review created and selected.');
		expect(body).toContain('review');
		expect(body).toContain('default');
		expect(body).toContain('reviewer');
		expect(body).toContain('front assistant');
		expect(body).toContain('scoped write specialist');
		expect(body).toContain('diff reviewer');
		expect(body).toContain('Save setup');
		expect(body).toContain('/tmp/storage/projects/proj-primary/teams/review/team.yaml');
	});
});
