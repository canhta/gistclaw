import { loadKnowledge } from '$lib/knowledge/load';
import { buildConfigKnowledgeHref, buildKnowledgeSearch } from '$lib/knowledge/query';
import { loadSettings } from '$lib/settings/load';
import { loadTeam } from '$lib/team/load';
import { loadWorkIndex } from '$lib/work/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	const [settings, team, work, knowledge] = await Promise.allSettled([
		loadSettings(fetch),
		loadTeam(fetch),
		loadWorkIndex(fetch),
		loadKnowledge(fetch, buildKnowledgeSearch(url.searchParams))
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
			: null;

	return {
		config: {
			settings: settings.status === 'fulfilled' ? settings.value : null,
			team: team.status === 'fulfilled' ? team.value : null,
			work: work.status === 'fulfilled' ? work.value : null,
			knowledge: knowledgeData
		}
	};
};
