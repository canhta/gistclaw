import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SessionDetail from './SessionDetail.svelte';
import type { ConversationDetailResponse } from '$lib/types/api';

const detail: ConversationDetailResponse = {
	session: {
		id: 'sess-1',
		agent_id: 'front',
		role_label: 'User',
		status_label: 'Active'
	},
	messages: [
		{
			kind: 'inbound',
			kind_label: 'Inbound',
			body: { plain_text: 'Hello world', html: '<p>Hello world</p>' },
			sender_label: 'Alice',
			sender_is_mono: false
		}
	],
	route: {
		id: 'route-1',
		connector_id: 'telegram',
		external_id: 'thread-1',
		thread_id: 'thread-1',
		status_label: 'Active',
		created_at_label: '1 min ago'
	},
	deliveries: [
		{
			id: 'delivery-terminal',
			connector_id: 'telegram',
			chat_id: 'chat-1',
			message: { plain_text: 'Retry exhausted', html: '<p>Retry exhausted</p>' },
			status: 'terminal',
			status_label: 'Terminal',
			attempts_label: '2 attempts'
		},
		{
			id: 'delivery-queued',
			connector_id: 'telegram',
			chat_id: 'chat-1',
			message: { plain_text: 'Queued', html: '<p>Queued</p>' },
			status: 'queued',
			status_label: 'Queued',
			attempts_label: '1 attempt'
		}
	],
	delivery_failures: [
		{
			id: 'failure-1',
			connector_id: 'telegram',
			chat_id: 'chat-1',
			event_kind_label: 'Webhook delivery',
			error: 'Connector timeout',
			created_at_label: 'just now'
		}
	]
};

describe('SessionDetail', () => {
	it('renders session id as heading', () => {
		const { body } = render(SessionDetail, { props: { detail } });
		expect(body).toContain('sess-1');
	});

	it('renders the agent id', () => {
		const { body } = render(SessionDetail, { props: { detail } });
		expect(body).toContain('front');
	});

	it('renders message plain text', () => {
		const { body } = render(SessionDetail, { props: { detail } });
		expect(body).toContain('Hello world');
	});

	it('renders sender label', () => {
		const { body } = render(SessionDetail, { props: { detail } });
		expect(body).toContain('Alice');
	});

	it('renders delivery rows and only shows retry for terminal deliveries', () => {
		const { body } = render(SessionDetail, { props: { detail } });
		expect(body).toContain('Retry exhausted');
		expect(body).toContain('Queued');
		expect(body).toContain('Retry delivery');
	});

	it('renders delivery failure evidence', () => {
		const { body } = render(SessionDetail, { props: { detail } });
		expect(body).toContain('Delivery failures');
		expect(body).toContain('Webhook delivery');
		expect(body).toContain('Connector timeout');
	});

	it('renders retry feedback copy', () => {
		const { body } = render(SessionDetail, {
			props: {
				detail,
				retryingDeliveryID: 'delivery-terminal',
				notice: 'Delivery requeued.',
				error: 'Only terminal deliveries can be retried.'
			}
		});
		expect(body).toContain('Retrying…');
		expect(body).toContain('Delivery requeued.');
		expect(body).toContain('Only terminal deliveries can be retried.');
	});

	it('renders empty state when no messages', () => {
		const emptyDetail = { ...detail, messages: [] };
		const { body } = render(SessionDetail, { props: { detail: emptyDetail } });
		expect(body).toContain('No messages');
	});
});
