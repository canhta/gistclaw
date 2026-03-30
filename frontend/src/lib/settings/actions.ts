import { requestJSON } from '$lib/http/client';
import type { SettingsActionResponse } from '$lib/types/api';

export interface SaveMachineSettingsInput {
	approval_mode?: string;
	host_access_mode?: string;
	per_run_token_budget?: string;
	daily_cost_cap_usd?: string;
	telegram_bot_token?: string;
	whatsapp_phone_number_id?: string;
	whatsapp_access_token?: string;
	whatsapp_verify_token?: string;
}

export function saveMachineSettings(
	fetcher: typeof fetch,
	updates: SaveMachineSettingsInput
): Promise<SettingsActionResponse> {
	return requestJSON<SettingsActionResponse>(fetcher, '/api/settings', {
		method: 'POST',
		headers: {
			'content-type': 'application/json'
		},
		body: JSON.stringify(updates)
	});
}
