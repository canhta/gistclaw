import { loadSettings } from '$lib/settings/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const data = await loadSettings(fetch);
		return {
			config: {
				settings: data
			}
		};
	} catch {
		return {
			config: {
				settings: null
			}
		};
	}
};
