import { loadOnboarding } from '$lib/onboarding/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	return {
		onboarding: await loadOnboarding(fetch)
	};
};
