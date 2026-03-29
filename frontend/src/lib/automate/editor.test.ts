import { describe, expect, it } from 'vitest';
import { buildAutomateCreateRequest, defaultAutomateEditorState } from './editor';

describe('automate editor helpers', () => {
	it('builds a cron schedule request from editor state', () => {
		const result = buildAutomateCreateRequest({
			...defaultAutomateEditorState(),
			name: ' Daily digest ',
			objective: ' Send a daily summary ',
			kind: 'cron',
			cronExpr: ' 0 9 * * * ',
			timezone: ' Asia/Ho_Chi_Minh '
		});

		expect(result).toEqual({
			ok: true,
			request: {
				name: 'Daily digest',
				objective: 'Send a daily summary',
				kind: 'cron',
				cron_expr: '0 9 * * *',
				timezone: 'Asia/Ho_Chi_Minh'
			}
		});
	});

	it('builds an every schedule request with an RFC3339 anchor timestamp', () => {
		const result = buildAutomateCreateRequest({
			...defaultAutomateEditorState(),
			name: 'Repository sweep',
			objective: 'Check the active project',
			kind: 'every',
			anchorAt: '2026-03-30T09:15',
			everyHours: '6'
		});

		expect(result).toEqual({
			ok: true,
			request: {
				name: 'Repository sweep',
				objective: 'Check the active project',
				kind: 'every',
				anchor_at: new Date('2026-03-30T09:15').toISOString(),
				every_hours: 6
			}
		});
	});

	it('returns validation errors when required cron fields are missing', () => {
		const result = buildAutomateCreateRequest({
			...defaultAutomateEditorState(),
			name: '',
			objective: '   ',
			kind: 'cron',
			cronExpr: ''
		});

		expect(result).toEqual({
			ok: false,
			errors: {
				name: 'Name is required.',
				objective: 'Objective is required.',
				cronExpr: 'Cron expression is required.'
			}
		});
	});

	it('returns validation errors when every schedule cadence is invalid', () => {
		const result = buildAutomateCreateRequest({
			...defaultAutomateEditorState(),
			name: 'Repository sweep',
			objective: 'Check the active project',
			kind: 'every',
			anchorAt: '',
			everyHours: '0'
		});

		expect(result).toEqual({
			ok: false,
			errors: {
				anchorAt: 'Start time is required.',
				everyHours: 'Repeat every hours must be greater than zero.'
			}
		});
	});
});
