import { describe, expect, it } from 'vitest';
import { buildConversationListSearch, buildSessionsPageHref } from './query';

describe('conversation query helpers', () => {
	it('keeps only supported conversation list query params', () => {
		const params = new URLSearchParams({
			tab: 'history',
			selected: 'sess-1',
			q: 'research',
			status: 'active',
			role: 'worker',
			connector_id: 'telegram',
			binding: 'bound',
			cursor: 'cursor-next',
			direction: 'next',
			limit: '25'
		});

		expect(buildConversationListSearch(params)).toBe(
			'q=research&role=worker&status=active&connector_id=telegram&binding=bound&cursor=cursor-next&direction=next&limit=25'
		);
	});

	it('maps api paging urls back to the sessions page and preserves the active tab', () => {
		expect(
			buildSessionsPageHref(
				'/api/conversations?status=active&cursor=cursor-next&direction=next',
				'tab=history'
			)
		).toBe('/sessions?status=active&cursor=cursor-next&direction=next&tab=history');
	});
});
