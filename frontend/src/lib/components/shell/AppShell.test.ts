import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import AppShell from './AppShell.svelte';

const baseProps = {
	navigation: [
		{ id: 'chat', label: 'Chat', href: '/chat' },
		{ id: 'approvals', label: 'Exec Approvals', href: '/approvals' },
		{ id: 'logs', label: 'Logs', href: '/logs' }
	],
	project: {
		active_id: 'proj-primary',
		active_name: 'starter-project',
		active_path: '/tmp/starter-project'
	},
	currentPath: '/approvals'
};

describe('AppShell', () => {
	it('does not render internal surface IDs in nav', () => {
		const { body } = render(AppShell, { props: baseProps });
		// Labels must appear
		expect(body).toContain('Chat');
		expect(body).toContain('Exec Approvals');
		expect(body).toContain('Logs');
		// Internal IDs must not appear as standalone nav text items
		expect(body).not.toContain('<span class="gc-machine">chat</span>');
		expect(body).not.toContain('<span class="gc-machine">approvals</span>');
	});

	it('renders the active project name in the shell', () => {
		const { body } = render(AppShell, { props: baseProps });
		expect(body).toContain('starter-project');
	});

	it('marks the active route with aria-current', () => {
		const { body } = render(AppShell, { props: baseProps });
		expect(body).toContain('aria-current="page"');
	});

	it('renders the desktop layout at xl breakpoint', () => {
		const { body } = render(AppShell, { props: baseProps });
		expect(body).toContain('xl:flex');
	});

	it('renders the mobile layout for small screens', () => {
		const { body } = render(AppShell, { props: baseProps });
		expect(body).toContain('xl:hidden');
	});

	it('renders GistClaw branding in mobile drawer', () => {
		const { body } = render(AppShell, {
			props: { ...baseProps, project: { ...baseProps.project, active_name: 'gistclaw' } }
		});
		expect(body).toContain('gistclaw');
	});

	it('renders inspector placeholder when no item selected', () => {
		const { body } = render(AppShell, { props: baseProps });
		expect(body).toContain('Select an item to inspect');
	});
});
