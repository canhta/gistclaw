import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ChannelsPage from './+page.svelte';

const nav = [{ id: 'channels', label: 'Channels', href: '/channels' }];

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: nav,
	onboarding: null,
	currentPath: '/channels',
	currentSearch: '',
	channels: {
		summary: {
			connector_count: 0,
			active_count: 0,
			pending_count: 0,
			retrying_count: 0,
			terminal_count: 0,
			restart_suggested_count: 0
		},
		access: {
			notice: '',
			settings: {
				machine: {
					storage_root: '/srv/gistclaw',
					approval_mode: 'prompt',
					approval_mode_label: 'Prompt',
					host_access_mode: 'standard',
					host_access_mode_label: 'Standard',
					admin_token: 'abcd1234****',
					per_run_token_budget: '50000',
					daily_cost_cap_usd: '5.00',
					rolling_cost_usd: 0.25,
					rolling_cost_label: '$0.25 in the last 24h',
					telegram_token: '12345678********',
					whatsapp_phone_number_id: 'phone-123',
					whatsapp_access_token: 'whatsapp********',
					whatsapp_verify_token: 'verify-s********',
					active_project_name: 'my-project',
					active_project_path: '/home/user/my-project',
					active_project_summary: 'my-project at /home/user/my-project'
				},
				access: {
					password_configured: true,
					other_active_devices: [],
					blocked_devices: []
				}
			},
			surfaces: [
				{
					id: 'telegram',
					name: 'Telegram',
					kind: 'connector',
					configured: true,
					active: true,
					credential_state: 'ready',
					credential_state_label: 'ready',
					summary: 'Bot token configured.',
					detail: 'Front agent assistant'
				},
				{
					id: 'whatsapp',
					name: 'WhatsApp',
					kind: 'connector',
					configured: true,
					active: true,
					credential_state: 'missing',
					credential_state_label: 'missing',
					summary: 'Connector is configured.',
					detail: 'Front agent assistant'
				}
			]
		},
		items: [],
		routes: {
			filters: {
				connector_id: 'telegram',
				status: 'all',
				query: '',
				limit: 10
			},
			items: [
				{
					id: 'route-1',
					session_id: 'sess-1',
					thread_id: 'thread-1',
					connector_id: 'telegram',
					account_id: 'acct-1',
					external_id: 'chat-1',
					status: 'inactive',
					status_label: 'Inactive',
					created_at_label: '2026-03-29 10:00 UTC',
					deactivated_at_label: '2026-03-29 11:00 UTC',
					deactivation_reason: 'deactivated',
					replaced_by_route_id: '',
					conversation_id: 'conv-1',
					agent_id: 'assistant',
					role: 'front',
					role_label: 'Front'
				}
			],
			paging: {
				has_next: true,
				has_prev: false,
				nextHref:
					'/channels?tab=settings&route_connector_id=telegram&route_status=all&route_limit=10&route_cursor=cursor-next&route_direction=next',
				prevHref: undefined
			}
		}
	}
};

describe('Channels page', () => {
	it('renders the Channels heading', () => {
		const { body } = render(ChannelsPage, { props: { data: baseData } });
		expect(body).toContain('Channels');
	});

	it('renders Status, Login, Settings tabs', () => {
		const { body } = render(ChannelsPage, { props: { data: baseData } });
		expect(body).toContain('Status');
		expect(body).toContain('Login');
		expect(body).toContain('Settings');
	});

	it('renders channel summary cards on the status tab', () => {
		const { body } = render(ChannelsPage, { props: { data: baseData } });
		expect(body).toContain('Live Channels');
		expect(body).toContain('Pending Deliveries');
		expect(body).toContain('Retrying Deliveries');
		expect(body).toContain('Terminal Deliveries');
	});

	it('renders empty state when no connectors', () => {
		const { body } = render(ChannelsPage, { props: { data: baseData } });
		expect(body).toContain('No channels connected');
	});

	it('renders connector row when channels are provided', () => {
		const data = {
			...baseData,
			channels: {
				access: baseData.channels.access,
				routes: baseData.channels.routes,
				summary: {
					connector_count: 1,
					active_count: 1,
					pending_count: 1,
					retrying_count: 0,
					terminal_count: 0,
					restart_suggested_count: 0
				},
				items: [
					{
						connector_id: 'telegram',
						state: 'active',
						state_label: 'Active',
						state_class: 'is-success',
						summary: 'Bot is connected',
						checked_at_label: '2 min ago',
						restart_suggested: false,
						pending_count: 1,
						retrying_count: 0,
						terminal_count: 0
					}
				]
			}
		};
		const { body } = render(ChannelsPage, { props: { data } });
		expect(body).toContain('telegram');
		expect(body).toContain('Bot is connected');
		expect(body).toContain('Pending deliveries');
	});

	it('renders login guidance when selected through search', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=login',
			channels: {
				...baseData.channels,
				summary: {
					connector_count: 2,
					active_count: 1,
					pending_count: 1,
					retrying_count: 0,
					terminal_count: 0,
					restart_suggested_count: 1
				},
				items: [
					{
						connector_id: 'telegram',
						state: 'active',
						state_label: 'Active',
						state_class: 'is-success',
						summary: 'Bot is connected',
						checked_at_label: '1 min ago',
						restart_suggested: false,
						pending_count: 1,
						retrying_count: 0,
						terminal_count: 0
					},
					{
						connector_id: 'whatsapp',
						state: 'degraded',
						state_label: 'Degraded',
						state_class: 'is-error',
						summary: 'Webhook token needs review',
						checked_at_label: '2 min ago',
						restart_suggested: true,
						pending_count: 0,
						retrying_count: 0,
						terminal_count: 0
					}
				]
			}
		};
		const { body } = render(ChannelsPage, { props: { data } });
		expect(body).toContain('Channel access board');
		expect(body).toContain('Telegram');
		expect(body).toContain('12345678********');
		expect(body).toContain('ready');
		expect(body).toContain('Save Telegram access');
		expect(body).toContain('Save WhatsApp access');
		expect(body).toContain('Telegram bot token');
		expect(body).toContain('WhatsApp phone number ID');
		expect(body).toContain('WhatsApp');
		expect(body).toContain('/webhooks/whatsapp');
		expect(body).not.toContain('Manage the bot token through the current machine settings flow.');
	});

	it('renders settings guidance when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=settings' };
		const { body } = render(ChannelsPage, { props: { data } });
		expect(body).toContain('Connector settings snapshot');
		expect(body).toContain('Masked Telegram token');
		expect(body).toContain('/webhooks/whatsapp');
		expect(body).toContain('Route directory');
		expect(body).toContain('Search routes');
		expect(body).toContain('Route status');
		expect(body).toContain('telegram');
		expect(body).toContain('chat-1');
		expect(body).toContain('Inactive');
		expect(body).toContain('Next route page');
		expect(body).not.toContain('Channel settings moved');
	});

	it('renders an access notice without hiding the channels boards', () => {
		const data = {
			...baseData,
			currentSearch: 'tab=login',
			channels: {
				...baseData.channels,
				access: {
					notice: 'Channel access details could not be loaded. Reload to retry.',
					settings: null,
					surfaces: []
				}
			}
		};
		const { body } = render(ChannelsPage, { props: { data } });
		expect(body).toContain('Channel access details could not be loaded. Reload to retry.');
		expect(body).toContain('Channel access board');
	});
});
