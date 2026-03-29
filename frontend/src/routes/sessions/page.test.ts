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
		summary: {
			session_count: 4,
			connector_count: 2,
			terminal_deliveries: 3
		},
		filters: {
			query: '',
			agent_id: '',
			role: '',
			status: '',
			connector_id: '',
			binding: ''
		},
		items: [],
		paging: { has_next: false, has_prev: false, nextHref: undefined, prevHref: undefined },
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

	it('renders session summary cards and filter controls', () => {
		const { body } = render(SessionsPage, { props: { data: baseData } });
		expect(body).toContain('4');
		expect(body).toContain('2');
		expect(body).toContain('3');
		expect(body).toContain('Filter sessions');
		expect(body).toContain('Search sessions');
		expect(body).toContain('Binding');
		expect(body).toContain('Connector');
		expect(body).toContain('Apply filters');
		expect(body).toContain('Clear filters');
	});

	it('renders the sessions table headings', () => {
		const { body } = render(SessionsPage, { props: { data: baseData } });
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
				...baseData.sessions,
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
				paging: { has_next: false, has_prev: false, nextHref: undefined, prevHref: undefined },
				runtimeConnectors: []
			}
		};
		const { body } = render(SessionsPage, { props: { data } });
		expect(body).toContain('sess-abc');
	});

	it('renders paging links when more sessions are available', () => {
		const data = {
			...baseData,
			sessions: {
				...baseData.sessions,
				paging: {
					has_next: true,
					has_prev: true,
					nextHref: '/sessions?cursor=next',
					prevHref: '/sessions?cursor=prev'
				}
			}
		};
		const { body } = render(SessionsPage, { props: { data } });
		expect(body).toContain('Previous Page');
		expect(body).toContain('/sessions?cursor=prev');
		expect(body).toContain('Next Page');
		expect(body).toContain('/sessions?cursor=next');
	});

	it('renders the history placeholder when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=history' };
		const { body } = render(SessionsPage, { props: { data } });
		expect(body).toContain('Session history');
		expect(body).toContain('not connected to a backend yet');
	});
});
