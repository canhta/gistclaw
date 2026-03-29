import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import OnboardingPage from './+page.svelte';

describe('Onboarding page', () => {
	it('renders project bind choices and starter task suggestions', () => {
		const { body } = render(OnboardingPage, {
			props: {
				data: {
					auth: {
						authenticated: true,
						password_configured: true,
						setup_required: false
					},
					onboarding: {
						completed: false,
						entry_href: '/onboarding',
						project: {
							active_id: 'proj-primary',
							active_name: 'starter-project',
							active_path: '/tmp/starter-project'
						},
						suggested_tasks: [
							{
								kind: 'explain',
								description: 'Explain what the internal package does',
								signal: 'directory "internal" matches known subsystem name'
							},
							{
								kind: 'review',
								description: 'Review changes in main.go',
								signal: 'code file "main.go" found in project root'
							}
						]
					},
					project: {
						active_id: 'proj-primary',
						active_name: 'starter-project',
						active_path: '/tmp/starter-project'
					},
					navigation: [
						{ id: 'chat', label: 'Chat', href: '/chat' },
						{ id: 'sessions', label: 'Sessions', href: '/sessions' }
					],
					currentPath: '/onboarding',
					currentSearch: ''
				}
			}
		});

		expect(body).toContain('Bind a repo and stage the first task');
		expect(body).toContain('Use the local starter project');
		expect(body).toContain('/tmp/starter-project');
		expect(body).toContain('Bind existing repo');
		expect(body).toContain('Create a fresh repo');
		expect(body).toContain('Explain what the internal package does');
		expect(body).toContain('Review changes in main.go');
		expect(body).toContain('Start preview run');
	});
});
