import { describe, expect, it, vi } from 'vitest';
import { saveMachineSettings } from './actions';

describe('saveMachineSettings', () => {
	it('posts partial machine settings to the settings endpoint', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			return new Response(
				JSON.stringify({
					notice: 'Machine settings updated.',
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
							whatsapp_phone_number_id: 'phone-987',
							whatsapp_access_token: 'whatsapp********',
							whatsapp_verify_token: 'verify-s********',
							active_project_name: 'my-project',
							active_project_path: '/srv/gistclaw/my-project',
							active_project_summary: 'my-project at /srv/gistclaw/my-project'
						},
						access: {
							password_configured: true,
							other_active_devices: [],
							blocked_devices: []
						}
					}
				}),
				{ status: 200, headers: { 'content-type': 'application/json' } }
			);
		});

		const response = await saveMachineSettings(fetcher, {
			telegram_bot_token: '87654321-token',
			whatsapp_phone_number_id: 'phone-987',
			whatsapp_access_token: 'whatsapp-secret',
			whatsapp_verify_token: 'verify-secret'
		});

		expect(fetcher).toHaveBeenCalledWith('/api/settings', {
			method: 'POST',
			headers: {
				accept: 'application/json',
				'content-type': 'application/json'
			},
			body: JSON.stringify({
				telegram_bot_token: '87654321-token',
				whatsapp_phone_number_id: 'phone-987',
				whatsapp_access_token: 'whatsapp-secret',
				whatsapp_verify_token: 'verify-secret'
			})
		});
		expect(response.notice).toBe('Machine settings updated.');
		expect(response.settings?.machine.whatsapp_phone_number_id).toBe('phone-987');
	});
});
