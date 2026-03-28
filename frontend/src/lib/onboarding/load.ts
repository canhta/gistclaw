import { requestJSON } from '$lib/http/client';
import type { OnboardingResponse } from '$lib/types/api';

export function loadOnboarding(fetcher: typeof fetch): Promise<OnboardingResponse> {
	return requestJSON<OnboardingResponse>(fetcher, '/api/onboarding');
}
