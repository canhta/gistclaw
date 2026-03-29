import { describe, expect, it } from 'vitest';
import { sectionForPath, sectionMap, sections } from './surfaces';

describe('surfaces', () => {
	it('exports all 12 section IDs', () => {
		const ids = sections.map((s) => s.id);
		expect(ids).toContain('chat');
		expect(ids).toContain('channels');
		expect(ids).toContain('instances');
		expect(ids).toContain('sessions');
		expect(ids).toContain('cron');
		expect(ids).toContain('skills');
		expect(ids).toContain('nodes');
		expect(ids).toContain('approvals');
		expect(ids).toContain('config');
		expect(ids).toContain('debug');
		expect(ids).toContain('logs');
		expect(ids).toContain('update');
		expect(ids.length).toBe(12);
	});

	it('does not export any of the old 8 section IDs', () => {
		const ids = sections.map((s) => s.id);
		const removed = [
			'work',
			'team',
			'knowledge',
			'recover',
			'conversations',
			'automate',
			'history',
			'settings'
		];
		for (const old of removed) {
			expect(ids).not.toContain(old);
		}
	});

	it('resolves section from path prefix', () => {
		expect(sectionForPath('/chat')?.id).toBe('chat');
		expect(sectionForPath('/approvals')?.id).toBe('approvals');
		expect(sectionForPath('/cron')?.id).toBe('cron');
		expect(sectionForPath('/sessions/123')?.id).toBe('sessions');
	});

	it('returns undefined for unknown paths', () => {
		expect(sectionForPath('/unknown-path')).toBeUndefined();
	});

	it('sectionMap provides O(1) lookup for all sections', () => {
		for (const section of sections) {
			expect(sectionMap[section.id]).toEqual(section);
		}
	});
});
