import { describe, expect, it } from 'vitest';
import { buildHistorySearch } from './query';

describe('history query helpers', () => {
	it('maps supported history page params to the history api contract', () => {
		const params = new URLSearchParams({
			tab: 'history',
			history_q: 'repair connector',
			history_status: 'failed',
			history_scope: 'all',
			history_limit: '10',
			status: 'active',
			role: 'worker'
		});

		expect(buildHistorySearch(params)).toBe('q=repair+connector&status=failed&scope=all&limit=10');
	});

	it('omits empty history filter params', () => {
		const params = new URLSearchParams({
			history_q: '   ',
			history_status: '',
			history_scope: '',
			history_limit: ''
		});

		expect(buildHistorySearch(params)).toBe('');
	});
});
