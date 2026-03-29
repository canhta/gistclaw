import { requestJSON } from '$lib/http/client';

const routeKeyMap = {
	route_connector_id: 'connector_id',
	route_status: 'status',
	route_q: 'q',
	route_limit: 'limit',
	route_cursor: 'cursor',
	route_direction: 'direction'
} as const;

interface RouteDirectoryItemRaw {
	ID?: string;
	SessionID?: string;
	ThreadID?: string;
	ConnectorID?: string;
	AccountID?: string;
	ExternalID?: string;
	Status?: string;
	CreatedAt?: string;
	DeactivatedAt?: string;
	DeactivationReason?: string;
	ReplacedByRouteID?: string;
	ConversationID?: string;
	AgentID?: string;
	Role?: string;
}

interface RouteDirectoryRawResponse {
	routes?: RouteDirectoryItemRaw[];
	next_cursor?: string;
	prev_cursor?: string;
	has_next?: boolean;
	has_prev?: boolean;
}

export interface ChannelRouteItem {
	id: string;
	session_id: string;
	thread_id: string;
	connector_id: string;
	account_id: string;
	external_id: string;
	status: string;
	status_label: string;
	created_at_label: string;
	deactivated_at_label?: string;
	deactivation_reason?: string;
	replaced_by_route_id?: string;
	conversation_id: string;
	agent_id: string;
	role: string;
	role_label: string;
}

export interface ChannelRouteDirectoryData {
	filters: {
		connector_id: string;
		status: string;
		query: string;
		limit: number;
	};
	items: ChannelRouteItem[];
	paging: {
		has_next: boolean;
		has_prev: boolean;
		nextHref?: string;
		prevHref?: string;
	};
}

export function buildChannelRoutesSearch(params: URLSearchParams): string {
	const next = new URLSearchParams();

	for (const [sourceKey, targetKey] of Object.entries(routeKeyMap)) {
		const value = params.get(sourceKey);
		if (value != null && value.trim() !== '') {
			next.set(targetKey, value);
		}
	}

	return next.toString();
}

export function buildChannelRoutesHref(
	cursor: string | undefined,
	direction: 'next' | 'prev',
	currentSearch = ''
): string | undefined {
	if (cursor == null || cursor.trim() === '') {
		return undefined;
	}

	const next = new URLSearchParams(currentSearch);
	next.set('tab', 'settings');
	next.set('route_cursor', cursor);
	next.set('route_direction', direction);

	const suffix = next.toString();
	return suffix === '' ? '/channels' : `/channels?${suffix}`;
}

export async function loadChannelRoutes(
	fetcher: typeof fetch,
	search = '',
	currentSearch = ''
): Promise<ChannelRouteDirectoryData> {
	const suffix = search.trim() === '' ? '' : `?${search}`;
	const raw = await requestJSON<RouteDirectoryRawResponse>(fetcher, `/api/routes${suffix}`);
	const params = new URLSearchParams(search);

	return {
		filters: {
			connector_id: params.get('connector_id')?.trim() ?? '',
			status: params.get('status')?.trim() ?? '',
			query: params.get('q')?.trim() ?? '',
			limit: parseRouteLimit(params.get('limit')?.trim() ?? '50')
		},
		items: (raw.routes ?? []).map(normalizeRouteItem),
		paging: {
			has_next: raw.has_next ?? false,
			has_prev: raw.has_prev ?? false,
			nextHref: buildChannelRoutesHref(raw.next_cursor, 'next', currentSearch),
			prevHref: buildChannelRoutesHref(raw.prev_cursor, 'prev', currentSearch)
		}
	};
}

function parseRouteLimit(value: string): number {
	const parsed = Number.parseInt(value, 10);
	return Number.isFinite(parsed) && parsed > 0 ? parsed : 50;
}

function normalizeRouteItem(item: RouteDirectoryItemRaw): ChannelRouteItem {
	return {
		id: item.ID ?? '',
		session_id: item.SessionID ?? '',
		thread_id: item.ThreadID ?? '',
		connector_id: item.ConnectorID ?? '',
		account_id: item.AccountID ?? '',
		external_id: item.ExternalID ?? '',
		status: item.Status ?? '',
		status_label: humanizeLabel(item.Status ?? ''),
		created_at_label: formatTimestamp(item.CreatedAt) ?? 'Not recorded',
		deactivated_at_label: formatTimestamp(item.DeactivatedAt),
		deactivation_reason: item.DeactivationReason ?? '',
		replaced_by_route_id: item.ReplacedByRouteID ?? '',
		conversation_id: item.ConversationID ?? '',
		agent_id: item.AgentID ?? '',
		role: item.Role ?? '',
		role_label: humanizeLabel(item.Role ?? '')
	};
}

function humanizeLabel(value: string): string {
	return value
		.trim()
		.split(/[_-]+/)
		.filter((part) => part !== '')
		.map((part) => part.charAt(0).toUpperCase() + part.slice(1))
		.join(' ');
}

function formatTimestamp(value: string | undefined): string | undefined {
	if (value == null || value.trim() === '') {
		return undefined;
	}

	return value.replace('T', ' ').replace('Z', ' UTC');
}
