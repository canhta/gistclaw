import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ConversationDetailPage from './+page.svelte';

describe('Conversation detail page', () => {
	it('renders the mailbox, route context, and delivery retry controls', () => {
		const { body } = render(ConversationDetailPage, {
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
					currentPath: '/conversations/session-1',
					currentSearch: '',
					conversation: {
						session: {
							id: 'session-1',
							agent_id: 'assistant',
							role_label: 'Lead agent',
							status_label: 'active'
						},
						messages: [
							{
								kind: 'assistant',
								kind_label: 'Assistant',
								body: { plain_text: 'Latest reply', html: '<p>Latest reply</p>' },
								sender_label: 'assistant',
								sender_is_mono: true,
								source_run_id: 'run-1'
							}
						],
						route: {
							id: 'route-1',
							connector_id: 'telegram',
							external_id: 'chat-1',
							thread_id: 'thread-1',
							status_label: 'active',
							created_at_label: '5m ago'
						},
						active_run_id: 'run-1',
						deliveries: [
							{
								id: 'delivery-1',
								connector_id: 'telegram',
								chat_id: 'chat-1',
								message: { plain_text: 'Outbound note', html: '<p>Outbound note</p>' },
								status: 'terminal',
								status_label: 'terminal',
								attempts_label: '3 attempts'
							}
						],
						delivery_failures: [
							{
								id: 'failure-1',
								connector_id: 'telegram',
								chat_id: 'chat-1',
								event_kind_label: 'Send failure',
								error: 'connector timed out',
								created_at_label: '1m ago'
							}
						]
					}
				}
			}
		});

		expect(body).toContain('Send operator message');
		expect(body).toContain('/work/run-1');
		expect(body).toContain('Retry delivery');
		expect(body).toContain('connector timed out');
		expect(body).toContain('Latest reply');
	});
});
