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
	chat: { runs: [], paging: { has_next: false, has_prev: false } }
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
	});

	it('renders empty state when no runs exist', () => {
		const { body } = render(ChatPage, { props: { data: baseData } });
		expect(body).toContain('No active session');
	});

	it('renders a run in the run list when runs are provided', () => {
		const data = {
			...baseData,
			chat: {
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
				paging: { has_next: false, has_prev: false }
			}
		};
		const { body } = render(ChatPage, { props: { data } });
		expect(body).toContain('Fix the authentication bug');
	});
});
