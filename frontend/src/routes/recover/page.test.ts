import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import RecoverPage from './+page.svelte';

describe('Recover page', () => {
	it('renders approvals and repair actions from the recover bench', () => {
		const { body } = render(RecoverPage, {
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
					navigation: [{ id: 'recover', label: 'Recover', href: '/recover' }],
					currentPath: '/recover',
					currentSearch: '',
					recover: {
						summary: {
							open_approvals: 2,
							pending_approvals: 1,
							connector_count: 1,
							active_routes: 1,
							terminal_deliveries: 1
						},
						approvals: [
							{
								id: 'approval-1',
								run_id: 'run-1',
								tool_name: 'bash',
								binding_summary: '/tmp/recover.txt',
								status: 'pending',
								status_label: 'pending',
								status_class: 'is-approval'
							}
						],
						approval_paging: {
							has_next: false,
							has_prev: false
						},
						repair: {
							connector_count: 1,
							filters: {
								query: '',
								connector_id: '',
								route_status: 'all',
								delivery_status: 'all',
								active_limit: 50,
								history_limit: 25,
								delivery_limit: 50
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
									state: 'degraded',
									state_label: 'Degraded',
									state_class: 'is-approval',
									summary: 'poll loop stale',
									restart_suggested: false
								}
							],
							active_routes: [
								{
									id: 'route-1',
									connector_id: 'telegram',
									external_id: 'chat-1',
									thread_id: 'thread-1',
									session_id: 'session-1',
									conversation_id: 'conv-1',
									agent_id: 'assistant',
									role_label: 'Lead agent',
									status_label: 'active'
								}
							],
							active_paging: { has_next: false, has_prev: false },
							route_history: [],
							history_paging: { has_next: false, has_prev: false },
							deliveries: [
								{
									id: 'delivery-1',
									run_id: 'run-1',
									session_id: 'session-1',
									connector_id: 'telegram',
									chat_id: 'chat-1',
									message: {
										plain_text: 'Retry this message.',
										html: '<p>Retry this message.</p>'
									},
									status: 'terminal',
									status_label: 'terminal',
									attempts_label: '3 attempts'
								}
							],
							delivery_paging: { has_next: false, has_prev: false }
						}
					}
				}
			}
		});

		expect(body).toContain('Put live operator work ahead of dead history');
		expect(body).toContain('/tmp/recover.txt');
		expect(body).toContain('Approve');
		expect(body).toContain('Deactivate route');
		expect(body).toContain('Retry delivery');
		expect(body).toContain('gc-action-warning');
		expect(body).toContain('/conversations/session-1');
	});
});
