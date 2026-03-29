import { loadKnowledge } from '$lib/knowledge/load';
import { buildConfigKnowledgeHref, buildKnowledgeSearch } from '$lib/knowledge/query';
import { loadSettings } from '$lib/settings/load';
import { loadTeam } from '$lib/team/load';
import type { KnowledgeResponse, TeamResponse } from '$lib/types/api';
import { loadWorkIndex } from '$lib/work/load';
import type { PageLoad } from './$types';

function fallbackTeamResponse(): TeamResponse {
	return {
		notice: 'Team controls could not be loaded. Reload to retry.',
		active_profile: {
			id: 'default',
			label: 'default',
			active: true
		},
		profiles: [
			{
				id: 'default',
				label: 'default',
				active: true
			}
		],
		team: {
			name: '',
			front_agent_id: '',
			member_count: 0,
			members: []
		}
	};
}

function fallbackKnowledgeResponse(search: string): KnowledgeResponse {
	const params = new URLSearchParams(search);
	const parsedLimit = Number.parseInt(params.get('limit') ?? '', 10);

	return {
		notice: 'Saved knowledge could not be loaded. Reload to retry.',
		headline: 'No saved knowledge is shaping work yet.',
		filters: {
			query: params.get('q') ?? '',
			scope: params.get('scope') ?? '',
			agent_id: params.get('agent_id') ?? '',
			limit: Number.isFinite(parsedLimit) && parsedLimit > 0 ? parsedLimit : 20
		},
		summary: {
			visible_count: 0
		},
		items: [],
		paging: {
			has_next: false,
			has_prev: false
		}
	};
}

export const load: PageLoad = async ({ fetch, url }) => {
	const knowledgeSearch = buildKnowledgeSearch(url.searchParams);
	const [settings, team, work, knowledge] = await Promise.allSettled([
		loadSettings(fetch),
		loadTeam(fetch),
		loadWorkIndex(fetch),
		loadKnowledge(fetch, knowledgeSearch)
	]);

	const currentSearch = url.searchParams.toString();
	const knowledgeData =
		knowledge.status === 'fulfilled'
			? {
					...knowledge.value,
					paging: {
						...knowledge.value.paging,
						nextHref: buildConfigKnowledgeHref(
							knowledge.value.paging.next_cursor,
							'next',
							currentSearch
						),
						prevHref: buildConfigKnowledgeHref(
							knowledge.value.paging.prev_cursor,
							'prev',
							currentSearch
						)
					}
				}
			: fallbackKnowledgeResponse(knowledgeSearch);

	return {
		config: {
			settings: settings.status === 'fulfilled' ? settings.value : null,
			team: team.status === 'fulfilled' ? team.value : fallbackTeamResponse(),
			work: work.status === 'fulfilled' ? work.value : null,
			knowledge: knowledgeData
		}
	};
};
