import { loadExtensionStatus } from '$lib/skills/load';
import type { ExtensionStatusResponse } from '$lib/types/api';
import type { PageLoad } from './$types';

const fallbackSkills: ExtensionStatusResponse = {
	summary: {
		shipped_surfaces: 0,
		configured_surfaces: 0,
		installed_tools: 0,
		ready_credentials: 0,
		missing_credentials: 0
	},
	surfaces: [],
	tools: []
};

export const load: PageLoad = async ({ fetch }) => {
	try {
		return {
			skills: await loadExtensionStatus(fetch)
		};
	} catch {
		return {
			skills: fallbackSkills
		};
	}
};
