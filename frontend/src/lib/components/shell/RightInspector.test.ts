import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import RightInspector from './RightInspector.svelte';

describe('RightInspector', () => {
	it('renders the inspector aside landmark', () => {
		const { body } = render(RightInspector, { props: {} });
		expect(body).toContain('<aside');
		expect(body).toContain('aria-label="Inspector"');
	});

	it('renders the empty state when no content is set', () => {
		const { body } = render(RightInspector, { props: {} });
		expect(body).toContain('Select an item to inspect');
	});

	it('has aria-live="polite" for dynamic updates', () => {
		const { body } = render(RightInspector, { props: {} });
		expect(body).toContain('aria-live="polite"');
	});
});
