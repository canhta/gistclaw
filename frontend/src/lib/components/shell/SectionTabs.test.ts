import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SectionTabs from './SectionTabs.svelte';

const tabs = [
	{ id: 'transcript', label: 'Transcript' },
	{ id: 'events', label: 'Run Events' },
	{ id: 'usage', label: 'Usage' }
];

describe('SectionTabs', () => {
	it('renders all tab labels', () => {
		const { body } = render(SectionTabs, { props: { tabs, activeTab: 'transcript' } });
		expect(body).toContain('Transcript');
		expect(body).toContain('Run Events');
		expect(body).toContain('Usage');
	});

	it('marks the active tab with aria-selected="true"', () => {
		const { body } = render(SectionTabs, { props: { tabs, activeTab: 'events' } });
		expect(body).toContain('aria-selected="true"');
	});

	it('marks inactive tabs with aria-selected="false"', () => {
		const { body } = render(SectionTabs, { props: { tabs, activeTab: 'transcript' } });
		// Two inactive tabs
		const matches = body.match(/aria-selected="false"/g);
		expect(matches?.length).toBe(2);
	});

	it('renders with role="tablist" container', () => {
		const { body } = render(SectionTabs, { props: { tabs, activeTab: 'transcript' } });
		expect(body).toContain('role="tablist"');
	});

	it('renders each tab with role="tab"', () => {
		const { body } = render(SectionTabs, { props: { tabs, activeTab: 'transcript' } });
		const matches = body.match(/role="tab"/g);
		expect(matches?.length).toBe(tabs.length);
	});

	it('sets tabindex=0 on active tab and -1 on inactive tabs', () => {
		const { body } = render(SectionTabs, { props: { tabs, activeTab: 'transcript' } });
		expect(body).toContain('tabindex="0"');
		const inactiveMatches = body.match(/tabindex="-1"/g);
		expect(inactiveMatches?.length).toBe(2);
	});
});
