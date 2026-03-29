import { describe, expect, it } from 'vitest';
import { defaultAnchorAt, serializeAnchorAt } from './automate-form';

describe('automate form helpers', () => {
	it('formats a local datetime value for the start time field', () => {
		const value = defaultAnchorAt(new Date('2026-03-29T08:17:42Z'));

		expect(value).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/);
		expect(Number.isNaN(Date.parse(value))).toBe(false);
	});

	it('keeps blank start time empty and serializes a typed value to ISO', () => {
		expect(serializeAnchorAt('')).toBe('');
		expect(serializeAnchorAt('2026-03-29T08:17')).toBe(new Date('2026-03-29T08:17').toISOString());
	});
});
