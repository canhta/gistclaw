import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch, search = ''): Parameters<typeof load>[0] {
	return {
		fetch: fetcher,
		url: new URL(`http://localhost/channels${search}`)
	} as unknown as Parameters<typeof load>[0];
}

describe('channels load', () => {
	it('loads a merged channels status board from conversations', async () => {
		const fetcher = vi.fn<typeof fetch>(
			async () =>
				new Response(
					JSON.stringify({
						summary: {
							session_count: 2,
							connector_count: 1,
							terminal_deliveries: 0
						},
						filters: {
							query: '',
							agent_id: '',
							role: '',
							status: '',
							connector_id: '',
							binding: ''
						},
						sessions: [],
						paging: { has_next: false, has_prev: false },
						health: [
							{
								connector_id: 'telegram',
								pending_count: 1,
								retrying_count: 0,
								terminal_count: 0,
								state_class: 'is-success'
							}
						],
						runtime_connectors: [
							{
								connector_id: 'telegram',
								state: 'active',
								state_label: 'Active',
								state_class: 'is-success',
								summary: 'Connected',
								checked_at_label: '1 min ago',
								restart_suggested: false
							}
						]
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				)
		);

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected channels load to return data');
		}

		expect(fetcher).toHaveBeenCalledWith('/api/conversations', expect.any(Object));
		expect(result.channels.summary).toEqual({
			connector_count: 1,
			active_count: 1,
			pending_count: 1,
			retrying_count: 0,
			terminal_count: 0,
			restart_suggested_count: 0
		});
		expect(result.channels.items).toEqual([
			expect.objectContaining({
				connector_id: 'telegram',
				state_label: 'Active',
				pending_count: 1,
				retrying_count: 0,
				terminal_count: 0
			})
		]);
		expect(result.channelsLoadError).toBe('');
		expect(result.channelAccessLoadError).toBe('');
		expect(result.channelRoutesLoadError).toBe('');
	});

	it('returns a load error when the channels status request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected channels load to return error data');
		}

		expect(result.channels).toBeNull();
		expect(result.channelsLoadError).toBe('Channel status could not be loaded. Reload to retry.');
		expect(result.channelAccessLoadError).toBe('');
		expect(result.channelRoutesLoadError).toBe('');
	});

	it('loads channel access data when the login tab is requested', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			if (input === '/api/conversations') {
				return new Response(
					JSON.stringify({
						summary: {
							session_count: 1,
							connector_count: 2,
							terminal_deliveries: 0
						},
						filters: {
							query: '',
							agent_id: '',
							role: '',
							status: '',
							connector_id: '',
							binding: ''
						},
						sessions: [],
						paging: { has_next: false, has_prev: false },
						health: [],
						runtime_connectors: [
							{
								connector_id: 'telegram',
								state: 'active',
								state_label: 'Active',
								state_class: 'is-success',
								summary: 'Bot is connected',
								checked_at_label: '1 min ago',
								restart_suggested: false
							},
							{
								connector_id: 'whatsapp',
								state: 'degraded',
								state_label: 'Degraded',
								state_class: 'is-error',
								summary: 'Webhook token needs review',
								checked_at_label: '2 min ago',
								restart_suggested: true
							}
						]
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			if (input === '/api/settings') {
				return new Response(
					JSON.stringify({
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
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			if (input === '/api/skills') {
				return new Response(
					JSON.stringify({
						summary: {
							shipped_surfaces: 2,
							configured_surfaces: 2,
							installed_tools: 1,
							ready_credentials: 1,
							missing_credentials: 1
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
						],
						tools: []
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			throw new Error(`unexpected request: ${String(input)}`);
		});

		const result = await load(makeLoadEvent(fetcher, '?tab=login'));

		if (!result) {
			throw new Error('expected channels load to return login access data');
		}

		expect(fetcher).toHaveBeenCalledWith('/api/conversations', expect.any(Object));
		expect(fetcher).toHaveBeenCalledWith('/api/settings', expect.any(Object));
		expect(fetcher).toHaveBeenCalledWith('/api/skills', expect.any(Object));
		expect(result.channels.access).toEqual({
			settings: expect.objectContaining({
				machine: expect.objectContaining({
					telegram_token: '12345678********',
					whatsapp_phone_number_id: 'phone-123',
					whatsapp_access_token: 'whatsapp********',
					whatsapp_verify_token: 'verify-s********'
				})
			}),
			surfaces: [
				expect.objectContaining({
					id: 'telegram',
					credential_state_label: 'ready'
				}),
				expect.objectContaining({
					id: 'whatsapp',
					credential_state_label: 'missing'
				})
			]
		});
		expect(result.channelAccessLoadError).toBe('');
	});

	it('returns an access load error when channel access reads fail', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			if (input === '/api/conversations') {
				return new Response(
					JSON.stringify({
						summary: {
							session_count: 1,
							connector_count: 0,
							terminal_deliveries: 0
						},
						filters: {
							query: '',
							agent_id: '',
							role: '',
							status: '',
							connector_id: '',
							binding: ''
						},
						sessions: [],
						paging: { has_next: false, has_prev: false },
						health: [],
						runtime_connectors: []
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher, '?tab=login'));

		if (!result) {
			throw new Error('expected channels load to return access error data');
		}

		expect(result.channels.access).toBeNull();
		expect(result.channelAccessLoadError).toBe(
			'Channel access details could not be loaded. Reload to retry.'
		);
	});

	it('loads route inventory when the settings tab is requested', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			if (input === '/api/conversations') {
				return new Response(
					JSON.stringify({
						summary: {
							session_count: 2,
							connector_count: 1,
							terminal_deliveries: 0
						},
						filters: {
							query: '',
							agent_id: '',
							role: '',
							status: '',
							connector_id: '',
							binding: ''
						},
						sessions: [],
						paging: { has_next: false, has_prev: false },
						health: [],
						runtime_connectors: []
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			if (input === '/api/routes?connector_id=telegram&status=all&limit=10') {
				return new Response(
					JSON.stringify({
						routes: [
							{
								ID: 'route-1',
								SessionID: 'sess-1',
								ThreadID: 'thread-1',
								ConnectorID: 'telegram',
								AccountID: 'acct-1',
								ExternalID: 'chat-1',
								Status: 'inactive',
								CreatedAt: '2026-03-29T10:00:00Z',
								DeactivatedAt: '2026-03-29T11:00:00Z',
								DeactivationReason: 'deactivated',
								ReplacedByRouteID: '',
								ConversationID: 'conv-1',
								AgentID: 'assistant',
								Role: 'front'
							}
						],
						has_next: true,
						has_prev: false,
						next_cursor: 'cursor-next'
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			if (input === '/api/settings') {
				return new Response(
					JSON.stringify({
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
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			if (input === '/api/skills') {
				return new Response(
					JSON.stringify({
						summary: {
							shipped_surfaces: 2,
							configured_surfaces: 2,
							installed_tools: 1,
							ready_credentials: 2,
							missing_credentials: 0
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
								credential_state: 'ready',
								credential_state_label: 'ready',
								summary: 'Webhook is configured.',
								detail: 'Front agent assistant'
							}
						],
						tools: []
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			throw new Error(`unexpected request: ${String(input)}`);
		});

		const result = await load(
			makeLoadEvent(
				fetcher,
				'?tab=settings&route_connector_id=telegram&route_status=all&route_limit=10'
			)
		);

		if (!result) {
			throw new Error('expected channels load to return route data');
		}

		expect(fetcher).toHaveBeenNthCalledWith(1, '/api/conversations', expect.any(Object));
		expect(fetcher).toHaveBeenNthCalledWith(
			2,
			'/api/routes?connector_id=telegram&status=all&limit=10',
			expect.any(Object)
		);
		expect(result.channels.routes.items[0]).toEqual(
			expect.objectContaining({
				id: 'route-1',
				connector_id: 'telegram',
				external_id: 'chat-1',
				status_label: 'Inactive'
			})
		);
		expect(result.channels.routes.paging.nextHref).toBe(
			'/channels?tab=settings&route_connector_id=telegram&route_status=all&route_limit=10&route_cursor=cursor-next&route_direction=next'
		);
		expect(result.channels.access.settings?.machine.telegram_token).toBe('12345678********');
		expect(result.channels.access.surfaces).toEqual([
			expect.objectContaining({ id: 'telegram' }),
			expect.objectContaining({ id: 'whatsapp' })
		]);
		expect(result.channelRoutesLoadError).toBe('');
		expect(result.channelAccessLoadError).toBe('');
	});

	it('returns a route load error when the route directory read fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			if (input === '/api/conversations') {
				return new Response(
					JSON.stringify({
						summary: {
							session_count: 2,
							connector_count: 1,
							terminal_deliveries: 0
						},
						filters: {
							query: '',
							agent_id: '',
							role: '',
							status: '',
							connector_id: '',
							binding: ''
						},
						sessions: [],
						paging: { has_next: false, has_prev: false },
						health: [],
						runtime_connectors: []
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			if (input === '/api/settings' || input === '/api/skills') {
				return new Response(
					JSON.stringify({
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
						},
						summary: {
							shipped_surfaces: 2,
							configured_surfaces: 2,
							installed_tools: 0,
							ready_credentials: 2,
							missing_credentials: 0
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
							}
						],
						tools: []
					}),
					{
						status: 200,
						headers: { 'content-type': 'application/json' }
					}
				);
			}

			throw new Error('boom');
		});

		const result = await load(
			makeLoadEvent(
				fetcher,
				'?tab=settings&route_connector_id=telegram&route_status=all&route_limit=10'
			)
		);

		if (!result) {
			throw new Error('expected channels load to return route error data');
		}

		expect(result.channels.routes).toBeNull();
		expect(result.channelRoutesLoadError).toBe(
			'Route directory could not be loaded. Reload to retry.'
		);
	});
});
