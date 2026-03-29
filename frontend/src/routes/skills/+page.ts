import { fallbackExtensionStatus, loadExtensionStatus } from '$lib/skills/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		return {
			skills: await loadExtensionStatus(fetch)
		};
	} catch {
		return {
			skills: fallbackExtensionStatus()
		};
	}
};
