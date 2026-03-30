import { loadDebugEvents, loadDebugRPC, loadDeliveryHealth } from '$lib/debug/load';
import { loadSettings } from '$lib/settings/load';
import { loadWorkIndex } from '$lib/work/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	const [settings, work, health, rpc, events] = await Promise.allSettled([
		loadSettings(fetch),
		loadWorkIndex(fetch),
		loadDeliveryHealth(fetch),
		loadDebugRPC(fetch, url.searchParams.get('probe')),
		loadDebugEvents(fetch, url.searchParams.get('run_id'))
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
						},
			rpc: rpc.status === 'fulfilled' ? rpc.value : null,
			events: events.status === 'fulfilled' ? events.value : null
		},
		debugRPCLoadError:
			rpc.status === 'fulfilled' ? '' : 'RPC probes could not be loaded. Reload to retry.'
	};
};
