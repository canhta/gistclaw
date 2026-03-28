import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import KnowledgePage from './+page.svelte';

describe('Knowledge page', () => {
	it('renders visible knowledge items with filters and edit actions', () => {
		const { body } = render(KnowledgePage, {
			props: {
				data: {
					auth: {
						authenticated: true,
						password_configured: true,
						setup_required: false
					},
					onboarding: {
						completed: true,
						entry_href: '/work'
					},
					project: {
						active_id: 'proj-primary',
						active_name: 'starter-project',
						active_path: '/tmp/starter-project'
					},
					navigation: [
						{ id: 'work', label: 'Work', href: '/work' },
						{ id: 'knowledge', label: 'Knowledge', href: '/knowledge' }
					],
					currentPath: '/knowledge',
					currentSearch: '',
					knowledge: {
						headline: 'Knowledge shaping future work in this project.',
						filters: {
							scope: '',
							agent_id: '',
							query: '',
							limit: 20
						},
						summary: {
							visible_count: 2
						},
						items: [
							{
								id: 'mem-1',
								agent_id: 'assistant',
								scope: 'local',
								content: 'captured operator preference',
								source: 'human',
								provenance: 'operator edit',
								confidence: 1,
								created_at_label: '2026-03-28 16:00:00 UTC',
								updated_at_label: '2026-03-28 16:10:00 UTC'
							},
							{
								id: 'mem-2',
								agent_id: 'patcher',
								scope: 'team',
								content: 'shared repo rule',
								source: 'model',
								provenance: 'run synthesis',
								confidence: 0.72,
								created_at_label: '2026-03-28 15:30:00 UTC',
								updated_at_label: '2026-03-28 15:50:00 UTC'
							}
						],
						paging: {
							has_next: false,
							has_prev: false,
							next_cursor: '',
							prev_cursor: ''
						}
					}
				}
			}
		});

		expect(body).toContain('Knowledge shaping future work in this project.');
		expect(body).toContain(
			'Save rules and facts in plain language so future work can follow them.'
		);
		expect(body).toContain('captured operator preference');
		expect(body).toContain('shared repo rule');
		expect(body).toContain('operator edit');
		expect(body).toContain('run synthesis');
		expect(body).toContain('Visible knowledge');
		expect(body).toContain('Save edit');
		expect(body).toContain('Forget item');
	});
});
