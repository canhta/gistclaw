import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SessionsPage from './+page.svelte';

const nav = [{ id: 'sessions', label: 'Sessions', href: '/sessions' }];

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: nav,
	onboarding: null,
	currentPath: '/sessions',
	currentSearch: '',
	sessions: {
		items: [],
		paging: { has_next: false, has_prev: false },
		runtimeConnectors: []
	}
};

describe('Sessions page', () => {
	it('renders the Sessions heading', () => {
		const { body } = render(SessionsPage, { props: { data: baseData } });
		expect(body).toContain('Sessions');
	});

	it('renders List, Overrides, History tabs', () => {
		const { body } = render(SessionsPage, { props: { data: baseData } });
		expect(body).toContain('List');
		expect(body).toContain('Overrides');
		expect(body).toContain('History');
	});

	it('renders a sessions search field and table headings', () => {
		const { body } = render(SessionsPage, { props: { data: baseData } });
		expect(body).toContain('Search sessions');
		expect(body).toContain('Session');
		expect(body).toContain('Agent');
		expect(body).toContain('Role');
		expect(body).toContain('Status');
		expect(body).toContain('Updated');
	});

	it('renders empty state when no sessions', () => {
		const { body } = render(SessionsPage, { props: { data: baseData } });
		expect(body).toContain('No sessions');
	});

	it('renders session row when sessions are provided', () => {
		const data = {
			...baseData,
			sessions: {
				items: [
					{
						id: 'sess-abc',
						conversation_id: 'conv-1',
						agent_id: 'front',
						role_label: 'User',
						status_label: 'Active',
						updated_at_label: '1 min ago'
					}
				],
				paging: { has_next: false, has_prev: false },
				runtimeConnectors: []
			}
		};
		const { body } = render(SessionsPage, { props: { data } });
		expect(body).toContain('sess-abc');
	});
});
