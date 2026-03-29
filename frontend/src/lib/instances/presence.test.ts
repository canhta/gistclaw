import { describe, expect, it } from 'vitest';
import { buildInstancePresenceData } from './presence';

const work = {
	active_project_name: 'my-project',
	active_project_path: '/home/user/my-project',
	queue_strip: {
		headline: '1 active run',
		root_runs: 1,
		worker_runs: 1,
		recovery_runs: 0,
		summary: {
			total: 2,
			pending: 0,
			active: 1,
			needs_approval: 1,
			completed: 0,
			failed: 0,
			interrupted: 0,
			root_status: 'active'
		}
	},
	paging: { has_next: false, has_prev: false },
	clusters: [
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
					id: 'run-child',
					objective: 'Inspect test failures',
					agent_id: 'reviewer',
					status: 'needs_approval',
					status_label: 'Needs approval',
					status_class: 'is-warning',
					model_display: 'gpt-5.4-mini',
					token_summary: '420 tokens',
					started_at_short: '10:01',
					started_at_exact: '2026-03-29 10:01',
					started_at_iso: '2026-03-29T10:01:00Z',
					last_activity_short: '10:04',
					last_activity_exact: '2026-03-29 10:04',
					last_activity_iso: '2026-03-29T10:04:00Z',
					depth: 1
				}
			],
			child_count: 1,
			child_count_label: '1 child run',
			blocker_label: 'Approval pending',
			has_children: true
		}
	]
};

const conversations = {
	summary: {
		session_count: 2,
		connector_count: 1,
		terminal_deliveries: 0
	},
	filters: {
		query: '',
		agent_id: '',
		role: '',
		status: '',
		connector_id: '',
		binding: ''
	},
	sessions: [],
	paging: { has_next: false, has_prev: false },
	health: [
		{
			connector_id: 'telegram',
			pending_count: 2,
			retrying_count: 1,
			terminal_count: 0,
			state_class: 'is-warning'
		}
	],
	runtime_connectors: [
		{
			connector_id: 'telegram',
			state: 'active',
			state_label: 'Active',
			state_class: 'is-success',
			summary: 'Presence beacons healthy',
			checked_at_label: '1 min ago',
			restart_suggested: false
		}
	]
};

describe('instance presence data', () => {
	it('builds a composed presence board from work and conversation signals', () => {
		const result = buildInstancePresenceData(work, conversations);

		expect(result.summary).toEqual({
			front_lane_count: 1,
			specialist_lane_count: 1,
			live_connector_count: 1,
			pending_delivery_count: 2
		});
		expect(result.lanes).toEqual([
			expect.objectContaining({
				id: 'run-root',
				kind: 'front',
				agent_id: 'assistant',
				status_label: 'Active'
			}),
			expect.objectContaining({
				id: 'run-child',
				kind: 'specialist',
				agent_id: 'reviewer',
				status_label: 'Needs approval'
			})
		]);
		expect(result.connectors).toEqual([
			expect.objectContaining({
				connector_id: 'telegram',
				pending_count: 2,
				retrying_count: 1
			})
		]);
		expect(result.sources).toEqual(
			expect.objectContaining({
				queue_headline: '1 active run',
				session_count: 2,
				connector_count: 1
			})
		);
	});

	it('returns empty presence data when no sources are available', () => {
		expect(buildInstancePresenceData(null, null)).toEqual({
			summary: {
				front_lane_count: 0,
				specialist_lane_count: 0,
				live_connector_count: 0,
				pending_delivery_count: 0
			},
			lanes: [],
			connectors: [],
			sources: {
				queue_headline: 'No work queue data',
				root_runs: 0,
				active_runs: 0,
				needs_approval_runs: 0,
				session_count: 0,
				connector_count: 0,
				terminal_deliveries: 0
			}
		});
	});
});
