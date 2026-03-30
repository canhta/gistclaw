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
						preview: {
							available: true,
							status_label: 'Ready to launch',
							detail: 'Start a preview run with the active project and current front assistant.',
							actions: [],
							checks: []
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
		expect(body).toContain('Preview readiness');
		expect(body).toContain('Ready to launch');
		expect(body).toContain('Start preview run');
	});

	it('renders a blocked preview state and disables preview actions', () => {
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
						preview: {
							available: false,
							status_label: 'Runtime unavailable',
							detail:
								'Preview runs are unavailable right now. Check the runtime configuration and try again.',
							actions: [
								{
									id: 'open-update',
									label: 'Open Update board',
									href: '/update'
								}
							],
							checks: [
								{
									id: 'ubuntu-doctor',
									label: 'Ubuntu doctor',
									detail: 'Check the shipped Ubuntu config before retrying preview runs.',
									command: 'gistclaw doctor --config /etc/gistclaw/config.yaml'
								},
								{
									id: 'ubuntu-inspect',
									label: 'Ubuntu runtime inspect',
									detail: 'Inspect the shipped Ubuntu daemon state from the CLI.',
									command: 'gistclaw inspect status --config /etc/gistclaw/config.yaml'
								}
							]
						},
						suggested_tasks: [
							{
								kind: 'explain',
								description: 'Explain what the internal package does',
								signal: 'directory "internal" matches known subsystem name'
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

		expect(body).toContain('Runtime unavailable');
		expect(body).toContain('Preview unavailable');
		expect(body).toContain('Preview recovery');
		expect(body).toContain('Open Update board');
		expect(body).toContain('gistclaw doctor --config /etc/gistclaw/config.yaml');
		expect(body).toContain('disabled');
	});
});
