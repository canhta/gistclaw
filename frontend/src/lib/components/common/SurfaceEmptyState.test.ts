import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SurfaceEmptyState from './SurfaceEmptyState.svelte';

describe('SurfaceEmptyState', () => {
	it('renders warm guidance with an optional follow-up action', () => {
		const { body } = render(SurfaceEmptyState, {
			props: {
				label: 'Idle machine',
				title: 'No live work yet',
				message: 'Launch the first task to open the graph and start building runtime evidence.',
				actionHref: '/work',
				actionLabel: 'Open Work'
			}
		});

		expect(body).toContain('Idle machine');
		expect(body).toContain('No live work yet');
		expect(body).toContain(
			'Launch the first task to open the graph and start building runtime evidence.'
		);
		expect(body).toContain('href="/work"');
		expect(body).toContain('Open Work');
	});
});
