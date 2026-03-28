import { describe, expect, it } from 'vitest';
import { surfaceByID, surfaceForPath } from './surfaces';

describe('surfaces', () => {
	it('returns inspector items for each surface', () => {
		const work = surfaceByID('work');
		expect(work.inspectorTitle).toBe('At a glance');
		expect(work.inspectorItems.length).toBeGreaterThan(0);

		const team = surfaceByID('team');
		expect(team.inspectorTitle).toBe('Setup');

		const recover = surfaceByID('recover');
		expect(recover.inspectorTitle).toBe('Needs attention');
	});

	it('resolves surface from path prefix', () => {
		expect(surfaceForPath('/work').id).toBe('work');
		expect(surfaceForPath('/conversations/123').id).toBe('conversations');
		expect(surfaceForPath('/unknown-path').id).toBe('work');
	});
});
