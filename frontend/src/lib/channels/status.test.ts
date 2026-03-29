import { describe, expect, it } from 'vitest';
import { buildChannelStatusData } from './status';

describe('buildChannelStatusData', () => {
	it('merges runtime connector state with delivery health and rolls up summary totals', () => {
		const result = buildChannelStatusData(
			[
				{
					connector_id: 'telegram',
					state: 'active',
					state_label: 'Active',
					state_class: 'is-success',
					summary: 'Bot connected',
					checked_at_label: '1 min ago',
					restart_suggested: false
				},
				{
					connector_id: 'whatsapp',
					state: 'error',
					state_label: 'Error',
					state_class: 'is-error',
					summary: 'QR expired',
					checked_at_label: '3 min ago',
					restart_suggested: true
				}
			],
			[
				{
					connector_id: 'telegram',
					pending_count: 2,
					retrying_count: 1,
					terminal_count: 0,
					state_class: 'is-success'
				},
				{
					connector_id: 'whatsapp',
					pending_count: 0,
					retrying_count: 0,
					terminal_count: 4,
					state_class: 'is-error'
				}
			]
		);

		expect(result.summary).toEqual({
			connector_count: 2,
			active_count: 1,
			pending_count: 2,
			retrying_count: 1,
			terminal_count: 4,
			restart_suggested_count: 1
		});
		expect(result.items).toEqual([
			expect.objectContaining({
				connector_id: 'whatsapp',
				terminal_count: 4,
				restart_suggested: true
			}),
			expect.objectContaining({
				connector_id: 'telegram',
				pending_count: 2,
				retrying_count: 1
			})
		]);
	});

	it('includes connectors that only appear in delivery health with a fallback runtime snapshot', () => {
		const result = buildChannelStatusData(
			[],
			[
				{
					connector_id: 'telegram',
					pending_count: 1,
					retrying_count: 0,
					terminal_count: 0,
					state_class: 'is-warning'
				}
			]
		);

		expect(result.items).toEqual([
			expect.objectContaining({
				connector_id: 'telegram',
				state: 'unknown',
				state_label: 'Unknown',
				summary: 'No runtime snapshot yet.',
				pending_count: 1,
				retrying_count: 0,
				terminal_count: 0
			})
		]);
	});
});
