const knowledgeKeyMap = {
	knowledge_q: 'q',
	knowledge_scope: 'scope',
	knowledge_agent_id: 'agent_id',
	knowledge_limit: 'limit',
	knowledge_cursor: 'cursor',
	knowledge_direction: 'direction'
} as const;

export function buildKnowledgeSearch(params: URLSearchParams): string {
	const next = new URLSearchParams();

	for (const [sourceKey, targetKey] of Object.entries(knowledgeKeyMap)) {
		const value = params.get(sourceKey);
		if (value != null && value.trim() !== '') {
			next.set(targetKey, value);
		}
	}

	return next.toString();
}

export function buildConfigKnowledgeHref(
	cursor: string | undefined,
	direction: 'next' | 'prev',
	currentSearch = ''
): string | undefined {
	if (cursor == null || cursor.trim() === '') {
		return undefined;
	}

	const next = new URLSearchParams(currentSearch);
	next.set('tab', 'general');
	next.set('knowledge_cursor', cursor);
	next.set('knowledge_direction', direction);

	const suffix = next.toString();
	return suffix === '' ? '/config' : `/config?${suffix}`;
}
