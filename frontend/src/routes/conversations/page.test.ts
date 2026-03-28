import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ConversationsPage from './+page.svelte';

describe('Conversations page', () => {
	it('renders sessions and connector health from the user communication surface', () => {
		const { body } = render(ConversationsPage, {
			props: {
				data: {
					auth: {
						authenticated: true,
						password_configured: true,
						setup_required: false
					},
					onboarding: {
						completed: true,
						entry_href: '/work'
					},
					project: {
						active_id: 'proj-primary',
						active_name: 'starter-project',
						active_path: '/tmp/starter-project'
					},
					navigation: [{ id: 'conversations', label: 'Conversations', href: '/conversations' }],
					currentPath: '/conversations',
					currentSearch: '',
					conversations: {
						summary: {
							session_count: 2,
							connector_count: 1,
							terminal_deliveries: 1
						},
						filters: {
							query: '',
							agent_id: '',
							role: '',
							status: '',
							connector_id: '',
							binding: ''
						},
						sessions: [
							{
								id: 'session-1',
								conversation_id: 'conv-1',
								agent_id: 'assistant',
								role_label: 'Lead agent',
								status_label: 'active',
								updated_at_label: '5m ago'
							}
						],
						paging: {
							has_next: false,
							has_prev: false
						},
						health: [
							{
								connector_id: 'telegram',
								pending_count: 0,
								retrying_count: 0,
								terminal_count: 1,
								state_class: 'is-error'
							}
						],
						runtime_connectors: [
							{
								connector_id: 'telegram',
								state: 'healthy',
								state_label: 'Healthy',
								state_class: 'is-active',
								summary: 'webhook activity recent',
								restart_suggested: false
							}
						]
					}
				}
			}
		});

		expect(body).toContain('See who is waiting on a reply');
		expect(body).toContain(
			'Check who is talking to GistClaw, which channels are healthy, and where replies need help.'
		);
		expect(body).toContain('/conversations/session-1');
		expect(body).toContain('webhook activity recent');
		expect(body).toContain('1 terminal');
	});
});
