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
		runtimeConnectors: [],
		selectedDetail: null,
		history: {
			summary: {
				run_count: 0,
				completed_runs: 0,
				recovery_runs: 0,
				approval_events: 0,
				delivery_outcomes: 0
			},
			filters: {
				query: '',
				status: '',
				scope: 'all',
				limit: 0
			},
			paging: { has_next: false, has_prev: false },
			runs: [],
			approvals: [],
			deliveries: []
		}
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

	it('renders override guidance when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=overrides' };
		const { body } = render(SessionsPage, { props: { data } });
		expect(body).toContain('Select a session from List');
		expect(body).toContain('Choose one conversation first');
		expect(body).toContain('List');
		expect(body).toContain('History');
	});

	it('renders session override controls when selected detail is available', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=overrides&session=sess-1',
			sessions: {
				...baseData.sessions,
				selectedDetail: {
					session: {
						id: 'sess-1',
						agent_id: 'front',
						role_label: 'User',
						status_label: 'Active'
					},
					messages: [],
					route: {
						id: 'route-1',
						connector_id: 'telegram',
						external_id: 'ext-1',
						thread_id: 'thread-1',
						status_label: 'Active',
						created_at_label: 'just now'
					},
					deliveries: [
						{
							id: 'delivery-1',
							connector_id: 'telegram',
							chat_id: 'chat-1',
							message: { plain_text: 'Retry exhausted', html: '<p>Retry exhausted</p>' },
							status: 'terminal',
							status_label: 'Terminal',
							attempts_label: '2 attempts'
						}
					],
					delivery_failures: [
						{
							id: 'failure-1',
							connector_id: 'telegram',
							chat_id: 'chat-1',
							event_kind_label: 'Webhook delivery',
							error: 'Connector timeout',
							created_at_label: 'just now'
						}
					]
				}
			}
		};
		const { body } = render(SessionsPage, { props: { data } });
		expect(body).toContain('Manage route and delivery overrides');
		expect(body).toContain('sess-1');
		expect(body).toContain('route-1');
		expect(body).toContain('Deactivate route');
		expect(body).toContain('Send route message');
		expect(body).toContain('Message body');
		expect(body).toContain('Wake the bound session with a manual operator message.');
		expect(body).toContain('Send message');
		expect(body).toContain('Retry delivery');
		expect(body).toContain('Webhook delivery');
	});

	it('renders route bind controls when the selected session has no active route', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=overrides&session=sess-1',
			sessions: {
				...baseData.sessions,
				runtimeConnectors: [
					{
						connector_id: 'telegram',
						state: 'active',
						state_label: 'Active',
						state_class: 'is-success',
						summary: 'Connected',
						checked_at_label: '1 min ago',
						restart_suggested: false
					}
				],
				selectedDetail: {
					session: {
						id: 'sess-1',
						agent_id: 'front',
						role_label: 'User',
						status_label: 'Active'
					},
					messages: [],
					deliveries: [],
					delivery_failures: []
				}
			}
		};
		const { body } = render(SessionsPage, { props: { data } });
		expect(body).toContain('Bind route');
		expect(body).toContain('Connector');
		expect(body).toContain('External ID');
		expect(body).toContain('Thread ID');
		expect(body).toContain('telegram');
	});

	it('renders session history evidence when selected through search', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=history',
			sessions: {
				...baseData.sessions,
				history: {
					summary: {
						run_count: 2,
						completed_runs: 1,
						recovery_runs: 1,
						approval_events: 1,
						delivery_outcomes: 1
					},
					filters: {
						query: 'repair',
						status: 'failed',
						scope: 'all',
						limit: 10
					},
					paging: { has_next: false, has_prev: false },
					runs: [
						{
							root: {
								id: 'run-123',
								objective: 'Repair connector backlog',
								agent_id: 'front',
								status: 'failed',
								status_label: 'Failed',
								status_class: 'is-error',
								model_display: 'gpt-5.4',
								token_summary: '1.2K tokens',
								started_at_short: '10:00',
								started_at_exact: '2026-03-29 10:00',
								started_at_iso: '2026-03-29T10:00:00Z',
								last_activity_short: '10:05',
								last_activity_exact: '2026-03-29 10:05',
								last_activity_iso: '2026-03-29T10:05:00Z',
								depth: 0
							},
							children: [],
							child_count: 0,
							child_count_label: 'No child runs',
							blocker_label: 'Needs operator review',
							has_children: false
						}
					],
					approvals: [
						{
							id: 'approval-1',
							run_id: 'run-123',
							tool_name: 'apply_patch',
							status: 'approved',
							status_label: 'Approved',
							resolved_by: 'operator',
							resolved_at_label: '1 min ago'
						}
					],
					deliveries: [
						{
							id: 'delivery-1',
							run_id: 'run-123',
							connector_id: 'telegram',
							chat_id: 'chat-1',
							status: 'terminal',
							status_label: 'Terminal',
							attempts_label: '2 attempts',
							last_attempt_at_label: 'just now',
							message_preview: 'Retry exhausted'
						}
					]
				}
			}
		};
		const { body } = render(SessionsPage, { props: { data } });
		expect(body).toContain('Project history');
		expect(body).toContain('Filter evidence');
		expect(body).toContain('Search evidence');
		expect(body).toContain('Run status');
		expect(body).toContain('Scope');
		expect(body).toContain('Limit');
		expect(body).toContain('Clear evidence filters');
		expect(body).toContain('Run filters only affect the run lane');
		expect(body).toContain('Repair connector backlog');
		expect(body).toContain('apply_patch');
		expect(body).toContain('telegram');
		expect(body).toContain('2 attempts');
	});

	it('renders empty session history state when no evidence is available', () => {
		const data = { ...baseData, currentSearch: 'tab=history' };
		const { body } = render(SessionsPage, { props: { data } });
		expect(body).toContain('Project history');
		expect(body).toContain('No run evidence yet');
	});
});
