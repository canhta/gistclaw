import { requestJSON } from '$lib/http/client';
import type { AutomateCreateRequest } from './editor';
import type { AutomateResponse, AutomateScheduleResponse } from '$lib/types/api';

export function loadAutomate(fetcher: typeof fetch): Promise<AutomateResponse> {
	return requestJSON<AutomateResponse>(fetcher, '/api/automate');
}

export async function createAutomateSchedule(
	fetcher: typeof fetch,
	input: AutomateCreateRequest
): Promise<AutomateScheduleResponse> {
	const response = await requestJSON<{ schedule: AutomateScheduleResponse }>(
		fetcher,
		'/api/automate',
		{
			method: 'POST',
			headers: {
				'content-type': 'application/json'
			},
			body: JSON.stringify(input)
		}
	);

	return response.schedule;
}
