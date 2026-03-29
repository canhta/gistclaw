import { describe, expect, it } from 'vitest';
import { summarizeModelUsage } from './models';

describe('work model summaries', () => {
	it('summarizes model usage across visible clusters', () => {
		const usage = summarizeModelUsage([
			{
				root: {
					id: 'run-root',
					objective: 'Review the repo',
					agent_id: 'assistant',
					status: 'active',
					status_label: 'Active',
					status_class: 'is-active',
					model_display: 'gpt-5.4',
					token_summary: '1K tokens',
					started_at_short: '10:00',
					started_at_exact: '2026-03-29 10:00',
					started_at_iso: '2026-03-29T10:00:00Z',
					last_activity_short: '10:05',
					last_activity_exact: '2026-03-29 10:05',
					last_activity_iso: '2026-03-29T10:05:00Z',
					depth: 0
				},
				children: [
					{
						id: 'run-child-1',
						objective: 'Inspect tests',
						agent_id: 'reviewer',
						status: 'completed',
						status_label: 'Completed',
						status_class: 'is-success',
						model_display: 'gpt-5.4-mini',
						token_summary: '400 tokens',
						started_at_short: '10:01',
						started_at_exact: '2026-03-29 10:01',
						started_at_iso: '2026-03-29T10:01:00Z',
						last_activity_short: '10:03',
						last_activity_exact: '2026-03-29 10:03',
						last_activity_iso: '2026-03-29T10:03:00Z',
						depth: 1
					},
					{
						id: 'run-child-2',
						objective: 'Patch docs',
						agent_id: 'patcher',
						status: 'completed',
						status_label: 'Completed',
						status_class: 'is-success',
						model_display: 'gpt-5.4',
						token_summary: '200 tokens',
						started_at_short: '10:02',
						started_at_exact: '2026-03-29 10:02',
						started_at_iso: '2026-03-29T10:02:00Z',
						last_activity_short: '10:04',
						last_activity_exact: '2026-03-29 10:04',
						last_activity_iso: '2026-03-29T10:04:00Z',
						depth: 1
					}
				],
				child_count: 2,
				child_count_label: '2 child runs',
				blocker_label: '',
				has_children: true
			}
		]);

		expect(usage).toEqual([
			{ model: 'gpt-5.4', count: 2 },
			{ model: 'gpt-5.4-mini', count: 1 }
		]);
	});

	it('ignores empty model labels', () => {
		const usage = summarizeModelUsage([
			{
				root: {
					id: 'run-root',
					objective: 'Review the repo',
					agent_id: 'assistant',
					status: 'active',
					status_label: 'Active',
					status_class: 'is-active',
					model_display: '',
					token_summary: '1K tokens',
					started_at_short: '10:00',
					started_at_exact: '2026-03-29 10:00',
					started_at_iso: '2026-03-29T10:00:00Z',
					last_activity_short: '10:05',
					last_activity_exact: '2026-03-29 10:05',
					last_activity_iso: '2026-03-29T10:05:00Z',
					depth: 0
				},
				children: [],
				child_count: 0,
				child_count_label: '0 child runs',
				blocker_label: '',
				has_children: false
			}
		]);

		expect(usage).toEqual([]);
	});
});
