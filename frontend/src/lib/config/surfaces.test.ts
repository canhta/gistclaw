import { describe, expect, it } from 'vitest';
import { surfaceByID } from './surfaces';

describe('surfaces', () => {
	it('uses shorter task-first descriptions for the shell header', () => {
		expect(surfaceByID('work').description).toBe(
			'Start tasks, watch progress, and step in when a run needs you.'
		);
		expect(surfaceByID('team').description).toBe(
			'Choose who leads, who helps, and how work gets handed off.'
		);
		expect(surfaceByID('knowledge').description).toBe(
			'Keep the facts and rules future work should follow.'
		);
		expect(surfaceByID('recover').description).toBe(
			'Clear approvals, retry failed work, and fix broken routes.'
		);
		expect(surfaceByID('conversations').description).toBe(
			'See active conversations, channel health, and replies that need help.'
		);
		expect(surfaceByID('automate').description).toBe(
			'Schedule recurring work and catch runs that may slip.'
		);
		expect(surfaceByID('history').description).toBe(
			'Review finished work, approvals, and delivery results.'
		);
		expect(surfaceByID('settings').description).toBe(
			'Manage browser access, active project, and daily work limits.'
		);
	});
});
