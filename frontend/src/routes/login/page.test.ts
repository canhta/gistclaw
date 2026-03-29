import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import LoginPage from './+page.svelte';

const baseData = {
	auth: {
		authenticated: false,
		password_configured: true,
		setup_required: false,
		login_reason: ''
	},
	project: {
		active_id: 'proj-1',
		active_name: 'my-project',
		active_path: '/home/user/my-project'
	},
	navigation: [
		{ id: 'chat', label: 'Chat', href: '/chat' },
		{ id: 'channels', label: 'Channels', href: '/channels' },
		{ id: 'sessions', label: 'Sessions', href: '/sessions' },
		{ id: 'approvals', label: 'Exec Approvals', href: '/approvals' }
	],
	onboarding: null,
	currentPath: '/login',
	currentSearch: ''
};

describe('Login page', () => {
	it('renders the page heading', () => {
		const { body } = render(LoginPage, { props: { data: baseData } });
		expect(body).toContain('Bring the local machine under operator control');
	});

	it('shows Chat, Sessions and Approvals section previews — not old Work/Recover/History labels', () => {
		const { body } = render(LoginPage, { props: { data: baseData } });
		expect(body).toContain('Chat');
		expect(body).toContain('Sessions');
		expect(body).toContain('Approvals');
		expect(body).not.toContain('WORK');
		expect(body).not.toContain('RECOVER');
		expect(body).not.toContain('HISTORY');
	});

	it('renders the password form', () => {
		const { body } = render(LoginPage, { props: { data: baseData } });
		expect(body).toContain('Authenticate this browser');
		expect(body).toContain('type="password"');
		expect(body).toContain('Open control deck');
	});

	it('shows setup required notice when setup_required is true', () => {
		const { body } = render(LoginPage, {
			props: {
				data: {
					...baseData,
					auth: { ...baseData.auth, setup_required: true }
				}
			}
		});
		expect(body).toContain('Setup required');
		expect(body).toContain('gistclaw auth set-password');
		expect(body).not.toContain('type="password"');
	});

	it('shows expired session notice when reason is expired', () => {
		const { body } = render(LoginPage, {
			props: {
				data: {
					...baseData,
					currentSearch: '?reason=expired'
				}
			}
		});
		expect(body).toContain('Your session expired');
	});

	it('shows blocked device notice when reason is blocked', () => {
		const { body } = render(LoginPage, {
			props: {
				data: {
					...baseData,
					currentSearch: '?reason=blocked'
				}
			}
		});
		expect(body).toContain('blocked');
	});
});
