import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SessionRow from './SessionRow.svelte';
import type { ConversationIndexItemResponse } from '$lib/types/api';

const session: ConversationIndexItemResponse = {
	id: 'sess-1',
	conversation_id: 'conv-1',
	agent_id: 'front',
	role_label: 'User',
	status_label: 'Active',
	updated_at_label: '2 min ago'
};

describe('SessionRow', () => {
	it('renders the session id as a stamp', () => {
		const { body } = render(SessionRow, { props: { session, selected: false } });
		expect(body).toContain('sess-1');
	});

	it('renders the agent id', () => {
		const { body } = render(SessionRow, { props: { session, selected: false } });
		expect(body).toContain('front');
	});

	it('renders the role label', () => {
		const { body } = render(SessionRow, { props: { session, selected: false } });
		expect(body).toContain('User');
	});

	it('renders the status label', () => {
		const { body } = render(SessionRow, { props: { session, selected: false } });
		expect(body).toContain('Active');
	});

	it('renders the updated_at_label', () => {
		const { body } = render(SessionRow, { props: { session, selected: false } });
		expect(body).toContain('2 min ago');
	});

	it('applies selected styling when selected is true', () => {
		const { body } = render(SessionRow, { props: { session, selected: true } });
		expect(body).toContain('gc-primary');
	});
});
