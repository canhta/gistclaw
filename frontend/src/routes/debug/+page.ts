import { loadDeliveryHealth } from '$lib/debug/load';
import { loadSettings } from '$lib/settings/load';
import { loadWorkIndex } from '$lib/work/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const [settings, work, health] = await Promise.allSettled([
		loadSettings(fetch),
		loadWorkIndex(fetch),
		loadDeliveryHealth(fetch)
	]);

	return {
		debug: {
			settings: settings.status === 'fulfilled' ? settings.value : null,
			work: work.status === 'fulfilled' ? work.value : null,
			health:
				health.status === 'fulfilled'
					? health.value
					: {
							connectors: [],
							runtime_connectors: []
						}
		}
	};
};
