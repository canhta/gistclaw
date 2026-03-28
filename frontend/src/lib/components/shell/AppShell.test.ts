import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import AppShell from './AppShell.svelte';

const baseProps = {
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
	inspectorTitle: 'Signal',
	inspectorItems: [
		{ label: 'Pending approvals', value: '3' },
		{ label: 'Route failures', value: '1' }
	]
};

describe('AppShell', () => {
	it('does not render the xl:block header panel', () => {
		const { body } = render(AppShell, { props: baseProps });
		expect(body).not.toContain('Active surface');
	});

	it('does not render internal surface IDs in nav', () => {
		const { body } = render(AppShell, { props: baseProps });
		// Labels must appear
		expect(body).toContain('Work');
		expect(body).toContain('Recover');
		expect(body).toContain('History');
		// Internal IDs must not appear as standalone nav text items
		// (they are still used in data attributes for icons, but not as visible span text)
		expect(body).not.toContain('<span class="gc-machine">work</span>');
		expect(body).not.toContain('<span class="gc-machine">recover</span>');
		expect(body).not.toContain('<span class="gc-machine">history</span>');
	});

	it('renders project name and path', () => {
		const { body } = render(AppShell, { props: baseProps });
		expect(body).toContain('starter-project');
		expect(body).toContain('/tmp/starter-project');
	});

	it('marks the active route with aria-current', () => {
		const { body } = render(AppShell, { props: baseProps });
		expect(body).toContain('aria-current="page"');
	});

	it('renders inspector items', () => {
		const { body } = render(AppShell, { props: baseProps });
		expect(body).toContain('Pending approvals');
		expect(body).toContain('Route failures');
	});

	it('renders the info toggle button for mobile', () => {
		const { body } = render(AppShell, { props: baseProps });
		expect(body).toContain('System info');
	});

	it('uses neutral shell labels', () => {
		const { body } = render(AppShell, {
			props: { ...baseProps, project: { ...baseProps.project, active_name: 'gistclaw' } }
		});
		expect(body).toContain('Control deck');
		expect(body).toContain('Repo workbench');
	});
});
