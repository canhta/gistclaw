import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import OccurrenceRow from './OccurrenceRow.svelte';
import type { AutomateOccurrenceResponse } from '$lib/types/api';

const occurrence: AutomateOccurrenceResponse = {
	id: 'occ-1',
	schedule_id: 'sched-1',
	schedule_name: 'Daily digest',
	status: 'completed',
	status_label: 'Completed',
	status_class: 'is-success',
	slot_at_label: 'Today 09:00',
	updated_at_label: '2 min ago'
};

const errorOccurrence: AutomateOccurrenceResponse = {
	...occurrence,
	id: 'occ-2',
	status: 'failed',
	status_label: 'Failed',
	status_class: 'is-error',
	error: 'timeout after 30s'
};

describe('OccurrenceRow', () => {
	it('renders the schedule name', () => {
		const { body } = render(OccurrenceRow, { props: { occurrence } });
		expect(body).toContain('Daily digest');
	});

	it('renders the status label', () => {
		const { body } = render(OccurrenceRow, { props: { occurrence } });
		expect(body).toContain('Completed');
	});

	it('renders the slot_at_label', () => {
		const { body } = render(OccurrenceRow, { props: { occurrence } });
		expect(body).toContain('Today 09:00');
	});

	it('renders error text when present', () => {
		const { body } = render(OccurrenceRow, { props: { occurrence: errorOccurrence } });
		expect(body).toContain('timeout after 30s');
	});

	it('does not render error section when no error', () => {
		const { body } = render(OccurrenceRow, { props: { occurrence } });
		expect(body).not.toContain('timeout after 30s');
	});
});
