import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import TranscriptRow from './TranscriptRow.svelte';

describe('TranscriptRow', () => {
	it('renders USER stamp and text for user role', () => {
		const { body } = render(TranscriptRow, {
			props: {
				row: {
					id: 'r1',
					role: 'user',
					text: 'Fix the bug please',
					timestamp: '2026-03-29T10:00:00Z'
				}
			}
		});
		expect(body).toContain('USER');
		expect(body).toContain('Fix the bug please');
	});

	it('renders AGENT stamp for agent role', () => {
		const { body } = render(TranscriptRow, {
			props: {
				row: {
					id: 'r2',
					role: 'agent',
					text: 'I will fix it.',
					isStreaming: false,
					toolCalls: [],
					timestamp: '2026-03-29T10:00:01Z'
				}
			}
		});
		expect(body).toContain('AGENT');
		expect(body).toContain('I will fix it.');
	});

	it('renders streaming indicator when isStreaming is true', () => {
		const { body } = render(TranscriptRow, {
			props: {
				row: {
					id: 'r3',
					role: 'agent',
					text: 'Working...',
					isStreaming: true,
					toolCalls: [],
					timestamp: '2026-03-29T10:00:02Z'
				}
			}
		});
		// Should show some visual streaming indicator (cursor or spinner class)
		expect(body).toContain('streaming');
	});

	it('renders NOTE stamp for system role', () => {
		const { body } = render(TranscriptRow, {
			props: {
				row: {
					id: 'r4',
					role: 'system',
					text: 'Operator injected a note',
					timestamp: '2026-03-29T10:00:03Z'
				}
			}
		});
		expect(body).toContain('NOTE');
		expect(body).toContain('Operator injected a note');
	});

	it('renders tool call count when agent row has tool calls', () => {
		const { body } = render(TranscriptRow, {
			props: {
				row: {
					id: 'r5',
					role: 'agent',
					text: 'Done.',
					isStreaming: false,
					toolCalls: [
						{
							id: 'tc-1',
							name: 'read_file',
							inputJSON: '{}',
							outputJSON: '{}',
							logs: [],
							status: 'completed' as const,
							expanded: false
						}
					],
					timestamp: '2026-03-29T10:00:04Z'
				}
			}
		});
		expect(body).toContain('read_file');
	});
});
