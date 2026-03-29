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
	deliveries: [],
	delivery_failures: []
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

	it('renders empty state when no messages', () => {
		const emptyDetail = { ...detail, messages: [] };
		const { body } = render(SessionDetail, { props: { detail: emptyDetail } });
		expect(body).toContain('No messages');
	});
});
