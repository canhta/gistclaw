import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ConnectorRow from './ConnectorRow.svelte';
import type { RecoverRuntimeHealthResponse } from '$lib/types/api';

const activeConnector: RecoverRuntimeHealthResponse = {
	connector_id: 'telegram',
	state: 'active',
	state_label: 'Active',
	state_class: 'is-success',
	summary: 'Connected',
	checked_at_label: '1 min ago',
	restart_suggested: false
};

const errorConnector: RecoverRuntimeHealthResponse = {
	connector_id: 'whatsapp',
	state: 'error',
	state_label: 'Error',
	state_class: 'is-error',
	summary: 'Connection lost',
	checked_at_label: '5 min ago',
	restart_suggested: true
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
});
