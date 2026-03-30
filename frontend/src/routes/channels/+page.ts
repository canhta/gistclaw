import { buildChannelRoutesSearch, loadChannelRoutes } from '$lib/channels/routes';
import { buildChannelStatusData } from '$lib/channels/status';
import { loadConversations } from '$lib/conversations/load';
import { loadExtensionStatus } from '$lib/skills/load';
import { loadSettings } from '$lib/settings/load';
import type { ExtensionSurfaceResponse, SettingsResponse } from '$lib/types/api';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	try {
		const requestedTab = url.searchParams.get('tab');
		const settingsRequested = requestedTab === 'settings';
		const accessRequested = requestedTab === 'login' || requestedTab === 'settings';
		const currentSearch = url.searchParams.toString();
		const [data, routes, settings, skills] = await Promise.allSettled([
			loadConversations(fetch),
			settingsRequested
				? loadChannelRoutes(fetch, buildChannelRoutesSearch(url.searchParams), currentSearch)
				: Promise.resolve(null),
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
		const access =
			accessRequested && settings.status === 'fulfilled' && skills.status === 'fulfilled'
				? {
						settings: settings.value as SettingsResponse,
						surfaces: (skills.value?.surfaces ?? []).filter(
							(surface: ExtensionSurfaceResponse) =>
								surface.kind === 'connector' &&
								(surface.id === 'telegram' || surface.id === 'whatsapp')
						)
					}
				: null;

		return {
			channels: {
				...channels,
				access,
				routes: settingsRequested && routes.status === 'fulfilled' ? routes.value : null
			},
			channelsLoadError: '',
			channelAccessLoadError:
				accessRequested && (settings.status !== 'fulfilled' || skills.status !== 'fulfilled')
					? 'Channel access details could not be loaded. Reload to retry.'
					: '',
			channelRoutesLoadError:
				settingsRequested && routes.status !== 'fulfilled'
					? 'Route directory could not be loaded. Reload to retry.'
					: ''
		};
	} catch {
		return {
			channels: null,
			channelsLoadError: 'Channel status could not be loaded. Reload to retry.',
			channelAccessLoadError: '',
			channelRoutesLoadError: ''
		};
	}
};
