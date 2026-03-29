import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ChatPage from './+page.svelte';

const nav = [
	{ id: 'chat', label: 'Chat', href: '/chat' },
	{ id: 'sessions', label: 'Sessions', href: '/sessions' }
];

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'proj-1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: nav,
	onboarding: null,
	currentPath: '/chat',
	currentSearch: '',
	chat: {
		projectName: 'my-project',
		projectPath: '/home/user/my-project',
		queue: {
			headline: 'No active runs',
			root_runs: 0,
			worker_runs: 0,
			recovery_runs: 0,
			summary: {
				total: 0,
				pending: 0,
				active: 0,
				needs_approval: 0,
				completed: 0,
				failed: 0,
				interrupted: 0,
				root_status: 'idle'
			}
		},
		runs: [],
		paging: { has_next: false, has_prev: false, nextHref: undefined, prevHref: undefined },
		selectedRunID: null,
		detail: null
	}
};

describe('Chat page', () => {
	it('renders the chat section heading', () => {
		const { body } = render(ChatPage, { props: { data: baseData } });
		expect(body).toContain('Chat');
	});

	it('renders tabs: Transcript, Run Events, Usage', () => {
		const { body } = render(ChatPage, { props: { data: baseData } });
		expect(body).toContain('Transcript');
		expect(body).toContain('Run Events');
		expect(body).toContain('Usage');
	});

	it('renders the composer textarea', () => {
		const { body } = render(ChatPage, { props: { data: baseData } });
		expect(body).toContain('textarea');
		expect(body).toContain('Type a message');
		expect(body).toContain('INJECT');
	});

	it('renders empty state when no runs exist', () => {
		const { body } = render(ChatPage, { props: { data: baseData } });
		expect(body).toContain('No active session');
	});

	it('renders queue strip summary cards', () => {
		const { body } = render(ChatPage, { props: { data: baseData } });
		expect(body).toContain('Root Runs');
		expect(body).toContain('Worker Runs');
		expect(body).toContain('Recovery Runs');
		expect(body).toContain('No active runs');
	});

	it('renders a run in the run list when runs are provided', () => {
		const data = {
			...baseData,
			chat: {
				...baseData.chat,
				runs: [
					{
						root: {
							id: 'run-abc123',
							objective: 'Fix the authentication bug',
							agent_id: 'agent-1',
							status: 'completed',
							status_label: 'Completed',
							status_class: 'is-success',
							model_display: 'claude-3-5',
							token_summary: '1.2K tokens',
							started_at_short: '10:00',
							started_at_exact: '2026-03-29 10:00:00',
							started_at_iso: '2026-03-29T10:00:00Z',
							last_activity_short: '10:05',
							last_activity_exact: '2026-03-29 10:05:00',
							last_activity_iso: '2026-03-29T10:05:00Z',
							depth: 0
						},
						children: [],
						child_count: 0,
						child_count_label: '',
						blocker_label: '',
						has_children: false
					}
				],
				selectedRunID: 'run-abc123',
				detail: {
					run: {
						id: 'run-abc123',
						short_id: 'run-abc123',
						objective_text: 'Fix the authentication bug',
						trigger_label: 'Chat',
						status: 'completed',
						status_label: 'Completed',
						status_class: 'is-success',
						state_label: 'Done',
						started_at_label: 'Started 10:00',
						last_activity_label: 'Last activity 10:05',
						model_display: 'claude-3-5',
						token_summary: '1.2K tokens',
						event_count: 8,
						turn_count: 2,
						stream_url: '/api/work/run-abc123/events',
						graph_url: '/api/work/run-abc123/graph',
						node_detail_url_template: '/api/work/run-abc123/nodes/{node_id}',
						dismissible: false
					},
					graph: {
						root_run_id: 'run-abc123',
						headline: 'Fix the authentication bug',
						summary: {
							total: 1,
							pending: 0,
							active: 0,
							needs_approval: 0,
							completed: 1,
							failed: 0,
							interrupted: 0,
							root_status: 'completed'
						},
						nodes: [],
						edges: [],
						active_path: []
					}
				}
			}
		};
		const { body } = render(ChatPage, { props: { data } });
		expect(body).toContain('Fix the authentication bug');
		expect(body).toContain('Started 10:00');
		expect(body).toContain('claude-3-5');
		expect(body).toContain('1.2K tokens');
	});

	it('renders paging links when more runs are available', () => {
		const data = {
			...baseData,
			chat: {
				...baseData.chat,
				paging: {
					has_next: true,
					has_prev: true,
					nextHref: '/chat?cursor=next',
					prevHref: '/chat?cursor=prev'
				}
			}
		};
		const { body } = render(ChatPage, { props: { data } });
		expect(body).toContain('Previous Page');
		expect(body).toContain('/chat?cursor=prev');
		expect(body).toContain('Next Page');
		expect(body).toContain('/chat?cursor=next');
	});

	it('renders the run events view when selected through search', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=run-events',
			chat: {
				...baseData.chat,
				runs: [
					{
						root: {
							id: 'run-abc123',
							objective: 'Fix the authentication bug',
							agent_id: 'agent-1',
							status: 'active',
							status_label: 'Active',
							status_class: 'is-active',
							model_display: 'claude-3-5',
							token_summary: '1.2K tokens',
							started_at_short: '10:00',
							started_at_exact: '2026-03-29 10:00:00',
							started_at_iso: '2026-03-29T10:00:00Z',
							last_activity_short: '10:05',
							last_activity_exact: '2026-03-29 10:05:00',
							last_activity_iso: '2026-03-29T10:05:00Z',
							depth: 0
						},
						children: [],
						child_count: 0,
						child_count_label: '',
						blocker_label: '',
						has_children: false
					}
				],
				selectedRunID: 'run-abc123'
			}
		};
		const { body } = render(ChatPage, { props: { data } });
		expect(body).toContain('Waiting for events');
		expect(body).toContain('run-ab');
	});
});
