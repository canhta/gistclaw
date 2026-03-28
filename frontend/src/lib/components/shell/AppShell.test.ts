import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import AppShell from './AppShell.svelte';

describe('AppShell', () => {
	it('renders project context, user-first navigation, and active route state', () => {
		const { body } = render(AppShell, {
			props: {
				navigation: [
					{ id: 'work', label: 'Work', href: '/work' },
					{ id: 'recover', label: 'Recover', href: '/recover' },
					{ id: 'history', label: 'History', href: '/history' }
				],
				project: {
					active_id: 'proj-primary',
					active_name: 'starter-project',
					active_path: '/tmp/starter-project'
				},
				currentPath: '/recover',
				title: 'Recover',
				description: 'Intervene in blocked work, evidence, and route repair.',
				inspectorTitle: 'Signal',
				inspectorItems: [
					{ label: 'Pending approvals', value: '3' },
					{ label: 'Route failures', value: '1' }
				]
			}
		});

		expect(body).toContain('starter-project');
		expect(body).toContain('/tmp/starter-project');
		expect(body).toContain('Work');
		expect(body).toContain('Recover');
		expect(body).toContain('History');
		expect(body).toContain('data-nav-icon="work"');
		expect(body).toContain('data-nav-icon="recover"');
		expect(body).toContain('aria-current="page"');
		expect(body).toContain('Intervene in blocked work, evidence, and route repair.');
		expect(body).toContain('Pending approvals');
		expect(body).toContain('Route failures');
		expect(body).toContain('data-shell-mobile-nav-strip');
		expect(body).toContain('data-shell-mobile-nav');
		expect(body).toContain('data-shell-mobile-signal');
	});

	it('uses neutral shell labels when the repo name matches the product brand', () => {
		const { body } = render(AppShell, {
			props: {
				navigation: [{ id: 'work', label: 'Work', href: '/work' }],
				project: {
					active_id: 'proj-gistclaw',
					active_name: 'gistclaw',
					active_path: '/Users/canh/Projects/OSS/gistclaw'
				},
				currentPath: '/work',
				title: 'Work',
				description: 'Steer current objectives, orchestration, and live machine signal.',
				inspectorTitle: 'Machine signal'
			}
		});

		expect(body).toContain('Control deck');
		expect(body).toContain('Bound repo');
		expect(body).toContain('Repo workbench');
		expect(body).not.toContain('>GistClaw</p>');
	});
});
