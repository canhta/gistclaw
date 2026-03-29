import { describe, expect, it } from 'vitest';
import { buildChatPageHref, buildWorkListSearch } from './query';

describe('work query helpers', () => {
	it('keeps only supported work list query params', () => {
		const params = new URLSearchParams({
			tab: 'run-events',
			run: 'run-123',
			cursor: 'next-cursor',
			direction: 'next',
			limit: '25'
		});

		expect(buildWorkListSearch(params)).toBe('cursor=next-cursor&direction=next&limit=25');
	});

	it('maps api paging urls back to chat and preserves tab and run', () => {
		expect(
			buildChatPageHref('/api/work?cursor=next-cursor&direction=next', 'tab=run-events&run=run-123')
		).toBe('/chat?cursor=next-cursor&direction=next&tab=run-events&run=run-123');
	});
});
