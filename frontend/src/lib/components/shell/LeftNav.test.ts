import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import LeftNav from './LeftNav.svelte';

const baseNav = [
	{ id: 'chat', label: 'Chat', href: '/chat' },
	{ id: 'approvals', label: 'Exec Approvals', href: '/approvals' },
	{ id: 'logs', label: 'Logs', href: '/logs' }
];

const baseProps = {
	navigation: baseNav,
	currentPath: '/chat',
	expanded: false,
	onToggle: () => {},
	theme: 'dark' as const,
	onToggleTheme: () => {}
};

describe('LeftNav', () => {
	it('marks the active route with aria-current', () => {
		const { body } = render(LeftNav, { props: baseProps });
		expect(body).toContain('aria-current="page"');
	});

	it('renders all nav item labels', () => {
		const { body } = render(LeftNav, { props: { ...baseProps, expanded: true } });
		expect(body).toContain('Chat');
		expect(body).toContain('Exec Approvals');
		expect(body).toContain('Logs');
	});

	it('hides labels in icon-only mode (collapsed)', () => {
		const { body } = render(LeftNav, { props: { ...baseProps, expanded: false } });
		// Labels are inside conditional block — in collapsed state span is not rendered
		expect(body).not.toContain('<span class="gc-stamp">Chat</span>');
	});

	it('renders the expand/collapse toggle button with keyboard shortcut hint', () => {
		const { body } = render(LeftNav, { props: baseProps });
		expect(body).toContain('aria-keyshortcuts="["');
	});

	it('renders navigation landmark', () => {
		const { body } = render(LeftNav, { props: baseProps });
		expect(body).toContain('aria-label="Primary navigation"');
	});

	it('applies active styling class for current path', () => {
		const { body } = render(LeftNav, { props: baseProps });
		expect(body).toContain('gc-primary');
	});
});
