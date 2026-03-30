import { loadExtensionStatus } from '$lib/skills/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		return {
			skills: await loadExtensionStatus(fetch),
			skillsLoadError: ''
		};
	} catch {
		return {
			skills: null,
			skillsLoadError: 'Skills status could not be loaded. Reload to retry.'
		};
	}
};
