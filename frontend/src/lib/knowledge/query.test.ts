import { describe, expect, it } from 'vitest';
import { buildConfigKnowledgeHref, buildKnowledgeSearch } from './query';

describe('knowledge query helpers', () => {
	it('maps supported config knowledge params to the knowledge api contract', () => {
		const params = new URLSearchParams({
			tab: 'general',
			knowledge_q: 'operator',
			knowledge_scope: 'local',
			knowledge_agent_id: 'assistant',
			knowledge_limit: '5'
		});

		expect(buildKnowledgeSearch(params)).toBe('q=operator&scope=local&agent_id=assistant&limit=5');
	});

	it('maps knowledge paging back to the config page and preserves active filters', () => {
		expect(
			buildConfigKnowledgeHref(
				'cursor-next',
				'next',
				'tab=general&knowledge_q=operator&knowledge_scope=local&knowledge_agent_id=assistant&knowledge_limit=5'
			)
		).toBe(
			'/config?tab=general&knowledge_q=operator&knowledge_scope=local&knowledge_agent_id=assistant&knowledge_limit=5&knowledge_cursor=cursor-next&knowledge_direction=next'
		);
	});
});
