import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ConnectorRow from './ConnectorRow.svelte';
import type { ChannelStatusItem } from '$lib/channels/status';

const activeConnector: ChannelStatusItem = {
	connector_id: 'telegram',
	state: 'active',
	state_label: 'Active',
	state_class: 'is-success',
	summary: 'Connected',
	checked_at_label: '1 min ago',
	restart_suggested: false,
	pending_count: 0,
	retrying_count: 0,
	terminal_count: 0
};

const errorConnector: ChannelStatusItem = {
	connector_id: 'whatsapp',
	state: 'error',
	state_label: 'Error',
	state_class: 'is-error',
	summary: 'Connection lost',
	checked_at_label: '5 min ago',
	restart_suggested: true,
	pending_count: 1,
	retrying_count: 2,
	terminal_count: 3
};

describe('ConnectorRow', () => {
	it('renders the connector id as a stamp label', () => {
		const { body } = render(ConnectorRow, { props: { connector: activeConnector } });
		expect(body).toContain('telegram');
	});

	it('renders the state label', () => {
		const { body } = render(ConnectorRow, { props: { connector: activeConnector } });
		expect(body).toContain('Active');
	});

	it('renders the summary text', () => {
		const { body } = render(ConnectorRow, { props: { connector: activeConnector } });
		expect(body).toContain('Connected');
	});

	it('renders restart suggestion badge when restart_suggested is true', () => {
		const { body } = render(ConnectorRow, { props: { connector: errorConnector } });
		expect(body).toContain('RESTART');
	});

	it('does not render restart badge when restart_suggested is false', () => {
		const { body } = render(ConnectorRow, { props: { connector: activeConnector } });
		expect(body).not.toContain('RESTART');
	});

	it('renders the checked_at_label', () => {
		const { body } = render(ConnectorRow, { props: { connector: activeConnector } });
		expect(body).toContain('1 min ago');
	});

	it('renders delivery queue counts', () => {
		const { body } = render(ConnectorRow, { props: { connector: errorConnector } });
		expect(body).toContain('Pending deliveries');
		expect(body).toContain('Retrying');
		expect(body).toContain('Terminal');
		expect(body).toContain('3');
	});

	it('renders a queue-clear message when there is no delivery pressure', () => {
		const { body } = render(ConnectorRow, { props: { connector: activeConnector } });
		expect(body).toContain('Queue is clear for this channel.');
	});

	it('renders an attention message when the channel has queue pressure', () => {
		const { body } = render(ConnectorRow, { props: { connector: errorConnector } });
		expect(body).toContain('Delivery queue needs attention on this channel.');
	});
});
