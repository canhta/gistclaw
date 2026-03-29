import { describe, expect, it } from 'vitest';
import {
	buildConversationListSearch,
	buildSessionDeliverySearch,
	buildSessionsDeliveryHref,
	buildSessionsPageHref
} from './query';

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

	it('builds a session delivery queue query from namespaced params', () => {
		const params = new URLSearchParams({
			tab: 'overrides',
			session: 'sess-1',
			delivery_q: 'telegram',
			delivery_status: 'terminal',
			delivery_cursor: 'delivery-next',
			delivery_direction: 'next',
			delivery_limit: '25'
		});

		expect(buildSessionDeliverySearch(params, 'sess-1')).toBe(
			'session_id=sess-1&q=telegram&status=terminal&cursor=delivery-next&direction=next&limit=25'
		);
	});

	it('maps delivery paging urls back to the overrides tab with namespaced params', () => {
		expect(
			buildSessionsDeliveryHref(
				'delivery-next',
				'next',
				'sess-1',
				'tab=overrides&session=sess-1&status=active',
				'25'
			)
		).toBe(
			'/sessions?tab=overrides&session=sess-1&status=active&delivery_cursor=delivery-next&delivery_direction=next&delivery_limit=25'
		);
	});
});
