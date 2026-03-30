import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import type { InstancesResponse } from '$lib/types/api';
import InstancesPage from './+page.svelte';

const baseInstances: InstancesResponse = {
	summary: {
		front_lane_count: 1,
		specialist_lane_count: 1,
		live_connector_count: 1,
		pending_delivery_count: 2
	},
	lanes: [
		{
			id: 'run-root',
			kind: 'front',
			agent_id: 'assistant',
			objective: 'Review the repo',
			status: 'active',
			status_label: 'Active',
			status_class: 'is-active',
			model_display: 'gpt-5.4',
			token_summary: '1K tokens',
			last_activity_short: '10:05'
		},
		{
			id: 'run-child',
			kind: 'specialist',
			agent_id: 'reviewer',
			objective: 'Inspect test failures',
			status: 'needs_approval',
			status_label: 'Needs approval',
			status_class: 'is-warning',
			model_display: 'gpt-5.4-mini',
			token_summary: '420 tokens',
			last_activity_short: '10:04'
		}
	],
	connectors: [
		{
			connector_id: 'telegram',
			state: 'healthy',
			state_label: 'Healthy',
			state_class: 'is-active',
			summary: 'Presence beacons healthy',
			checked_at_label: '1 min ago',
			restart_suggested: false,
			pending_count: 2,
			retrying_count: 1,
			terminal_count: 0
		}
	],
	sources: {
		queue_headline: '1 active run',
		root_runs: 1,
		active_runs: 1,
		needs_approval_runs: 1,
		session_count: 2,
		connector_count: 1,
		terminal_deliveries: 0
	}
};

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: [{ id: 'instances', label: 'Instances', href: '/instances' }],
	onboarding: null,
	currentPath: '/instances',
	currentSearch: '',
	instances: baseInstances
};

describe('Instances page', () => {
	it('renders the Instances heading', () => {
		const { body } = render(InstancesPage, { props: { data: baseData } });
		expect(body).toContain('Instances');
	});

	it('renders Presence and Details tabs', () => {
		const { body } = render(InstancesPage, { props: { data: baseData } });
		expect(body).toContain('Presence');
		expect(body).toContain('Details');
	});

	it('renders presence summary cards and project context', () => {
		const { body } = render(InstancesPage, { props: { data: baseData } });
		expect(body).toContain('Front Lanes');
		expect(body).toContain('Specialist Lanes');
		expect(body).toContain('Live Connectors');
		expect(body).toContain('Project');
		expect(body).toContain('my-project');
		expect(body).toContain('/home/user/my-project');
	});

	it('renders runtime presence board by default', () => {
		const { body } = render(InstancesPage, { props: { data: baseData } });
		expect(body).toContain('Runtime presence board');
		expect(body).toContain('inventory feed');
		expect(body).toContain('assistant');
		expect(body).toContain('reviewer');
		expect(body).toContain('Review the repo');
		expect(body).toContain('Inspect test failures');
		expect(body).toContain('Presence beacons healthy');
		expect(body).toContain('telegram');
	});

	it('renders source details when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=details' };
		const { body } = render(InstancesPage, { props: { data } });
		expect(body).toContain('Source surfaces');
		expect(body).toContain('instance inventory API');
		expect(body).toContain('1 active run');
		expect(body).toContain('2');
		expect(body).toContain('Chat');
		expect(body).toContain('Sessions');
		expect(body).toContain('Channels');
	});

	it('renders an empty presence state when no lanes or connectors are available', () => {
		const data = {
			...baseData,
			instances: {
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
			}
		};
		const { body } = render(InstancesPage, { props: { data } });
		expect(body).toContain('No active runtime presence');
		expect(body).toContain('Run work from Chat or connect a channel');
	});
});
