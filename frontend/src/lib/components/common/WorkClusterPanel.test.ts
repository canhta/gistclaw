import { render } from 'svelte/server';
import { describe, it, expect } from 'vitest';
import WorkClusterPanel from './WorkClusterPanel.svelte';
import type { WorkClusterResponse } from '$lib/types/api';

const runningCluster: WorkClusterResponse = {
	root: {
		id: 'run-1',
		objective: 'Audit and patch issue #42',
		agent_id: 'front_assistant',
		status: 'active',
		status_label: 'Running',
		status_class: 'is-active',
		model_display: 'claude-opus',
		token_summary: '12k tokens',
		started_at_short: '5 min ago',
		started_at_exact: '',
		started_at_iso: '',
		last_activity_short: '1 min ago',
		last_activity_exact: '',
		last_activity_iso: '',
		depth: 0
	},
	children: [
		{
			id: 'run-2',
			objective: 'Research the issue history',
			agent_id: 'researcher',
			status: 'active',
			status_label: 'Running',
			status_class: 'is-active',
			model_display: 'claude-haiku',
			token_summary: '3k tokens',
			started_at_short: '3 min ago',
			started_at_exact: '',
			started_at_iso: '',
			last_activity_short: '30 sec ago',
			last_activity_exact: '',
			last_activity_iso: '',
			depth: 1
		}
	],
	child_count: 1,
	child_count_label: '1 worker',
	blocker_label: '',
	has_children: true
};

const approvalCluster: WorkClusterResponse = {
	...runningCluster,
	root: { ...runningCluster.root, status_class: 'is-approval', status_label: 'Needs approval' }
};

describe('WorkClusterPanel', () => {
	it('renders the root agent objective', () => {
		const { body } = render(WorkClusterPanel, { props: { cluster: runningCluster } });
		expect(body).toContain('Audit and patch issue #42');
	});

	it('renders worker agents', () => {
		const { body } = render(WorkClusterPanel, { props: { cluster: runningCluster } });
		expect(body).toContain('Research the issue history');
	});

	it('applies warning border class when approval-blocked', () => {
		const { body } = render(WorkClusterPanel, { props: { cluster: approvalCluster } });
		expect(body).toContain('border-[var(--gc-orange)]');
	});

	it('renders a link to the run', () => {
		const { body } = render(WorkClusterPanel, { props: { cluster: runningCluster } });
		expect(body).toContain('run-1');
	});

	it('formats agent_id as readable label', () => {
		const { body } = render(WorkClusterPanel, { props: { cluster: runningCluster } });
		expect(body).toContain('Front assistant');
	});
});
