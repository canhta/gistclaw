import { buildChannelRoutesSearch, loadChannelRoutes } from '$lib/channels/routes';
import { buildChannelStatusData } from '$lib/channels/status';
import { loadConversations } from '$lib/conversations/load';
import { loadExtensionStatus } from '$lib/skills/load';
import { loadSettings } from '$lib/settings/load';
import type { ExtensionSurfaceResponse, SettingsResponse } from '$lib/types/api';
import type { PageLoad } from './$types';

const emptyRoutes = {
	filters: {
		connector_id: '',
		status: '',
		query: '',
		limit: 50
	},
	items: [],
	paging: {
		has_next: false,
		has_prev: false,
		nextHref: undefined,
		prevHref: undefined
	}
};

const emptyAccess = {
	notice: '',
	settings: null as SettingsResponse | null,
	surfaces: [] as ExtensionSurfaceResponse[]
};

export const load: PageLoad = async ({ fetch, url }) => {
	try {
		const requestedTab = url.searchParams.get('tab');
		const settingsRequested = requestedTab === 'settings';
		const accessRequested = requestedTab === 'login' || requestedTab === 'settings';
		const currentSearch = url.searchParams.toString();
		const [data, routes, settings, skills] = await Promise.allSettled([
			loadConversations(fetch),
			settingsRequested
				? loadChannelRoutes(fetch, buildChannelRoutesSearch(url.searchParams), currentSearch).catch(
						() => emptyRoutes
					)
				: Promise.resolve(emptyRoutes),
			accessRequested ? loadSettings(fetch) : Promise.resolve(null),
			accessRequested ? loadExtensionStatus(fetch) : Promise.resolve(null)
		]);
		if (data.status !== 'fulfilled') {
			throw data.reason;
		}

		const channels = buildChannelStatusData(
			data.value.runtime_connectors ?? [],
			data.value.health ?? []
		);
		const access = !accessRequested
			? emptyAccess
			: {
					notice:
						settings.status === 'fulfilled' && skills.status === 'fulfilled'
							? ''
							: 'Channel access details could not be loaded. Reload to retry.',
					settings: settings.status === 'fulfilled' ? settings.value : null,
					surfaces:
						skills.status === 'fulfilled'
							? (skills.value?.surfaces ?? []).filter(
									(surface) =>
										surface.kind === 'connector' &&
										(surface.id === 'telegram' || surface.id === 'whatsapp')
								)
							: []
				};

		return {
			channels: {
				...channels,
				access,
				routes: routes.status === 'fulfilled' ? routes.value : emptyRoutes
			}
		};
	} catch {
		return {
			channels: {
				summary: {
					connector_count: 0,
					active_count: 0,
					pending_count: 0,
					retrying_count: 0,
					terminal_count: 0,
					restart_suggested_count: 0
				},
				items: [],
				access: emptyAccess,
				routes: emptyRoutes
			}
		};
	}
};
