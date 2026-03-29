import { describe, expect, it } from 'vitest';
import { applyEvent, makeTranscriptState } from './transcript.svelte';
import type { ReplayDeltaEnvelope } from '$lib/http/events';

function evt(kind: string, payload: unknown, id = 'evt-1'): ReplayDeltaEnvelope {
	return {
		event_id: id,
		run_id: 'run-1',
		kind,
		payload,
		occurred_at: '2026-03-29T10:00:00Z'
	};
}

describe('makeTranscriptState', () => {
	it('starts with empty rows and idle status', () => {
		const state = makeTranscriptState();
		expect(state.rows).toHaveLength(0);
		expect(state.runStatus).toBe('idle');
		expect(state.activeRunId).toBeNull();
	});
});

describe('applyEvent — run lifecycle', () => {
	it('run_started creates a user row with the objective', () => {
		const state = makeTranscriptState();
		applyEvent(state, evt('run_started', { objective: 'Fix the bug' }));
		expect(state.rows).toHaveLength(1);
		expect(state.rows[0].role).toBe('user');
		expect(state.rows[0].text).toBe('Fix the bug');
		expect(state.runStatus).toBe('active');
	});

	it('run_completed sets status to completed', () => {
		const state = makeTranscriptState();
		applyEvent(state, evt('run_started', { objective: 'Task' }));
		applyEvent(state, evt('run_completed', { input_tokens: 100, output_tokens: 200 }));
		expect(state.runStatus).toBe('completed');
		expect(state.tokenSummary).toEqual({ inputTokens: 100, outputTokens: 200 });
	});

	it('run_interrupted sets status to interrupted', () => {
		const state = makeTranscriptState();
		applyEvent(state, evt('run_started', { objective: 'Task' }));
		applyEvent(state, evt('run_interrupted', {}));
		expect(state.runStatus).toBe('interrupted');
	});

	it('run_failed sets status to failed', () => {
		const state = makeTranscriptState();
		applyEvent(state, evt('run_started', { objective: 'Task' }));
		applyEvent(state, evt('run_failed', {}));
		expect(state.runStatus).toBe('failed');
	});
});

describe('applyEvent — agent turn streaming', () => {
	it('turn_delta creates an agent row with streaming text', () => {
		const state = makeTranscriptState();
		applyEvent(state, evt('run_started', { objective: 'Task' }));
		applyEvent(state, evt('turn_delta', { text: 'Hello ' }, 'evt-2'));
		expect(state.rows).toHaveLength(2);
		const agentRow = state.rows[1];
		expect(agentRow.role).toBe('agent');
		if (agentRow.role === 'agent') {
			expect(agentRow.text).toBe('Hello ');
			expect(agentRow.isStreaming).toBe(true);
		}
	});

	it('consecutive turn_delta events accumulate text in same row', () => {
		const state = makeTranscriptState();
		applyEvent(state, evt('run_started', { objective: 'Task' }));
		applyEvent(state, evt('turn_delta', { text: 'Hello ' }, 'evt-2'));
		applyEvent(state, evt('turn_delta', { text: 'world' }, 'evt-3'));
		expect(state.rows).toHaveLength(2);
		const agentRow = state.rows[1];
		if (agentRow.role === 'agent') {
			expect(agentRow.text).toBe('Hello world');
			expect(agentRow.isStreaming).toBe(true);
		}
	});

	it('turn_completed finalizes the agent row', () => {
		const state = makeTranscriptState();
		applyEvent(state, evt('run_started', { objective: 'Task' }));
		applyEvent(state, evt('turn_delta', { text: 'Hello ' }, 'evt-2'));
		applyEvent(
			state,
			evt('turn_completed', { content: 'Hello world', input_tokens: 10, output_tokens: 5 }, 'evt-3')
		);
		const agentRow = state.rows[1];
		if (agentRow.role === 'agent') {
			expect(agentRow.text).toBe('Hello world');
			expect(agentRow.isStreaming).toBe(false);
		}
	});

	it('turn_completed without prior turn_delta creates agent row', () => {
		const state = makeTranscriptState();
		applyEvent(state, evt('run_started', { objective: 'Task' }));
		applyEvent(state, evt('turn_completed', { content: 'Done.' }, 'evt-2'));
		expect(state.rows).toHaveLength(2);
		const agentRow = state.rows[1];
		if (agentRow.role === 'agent') {
			expect(agentRow.text).toBe('Done.');
			expect(agentRow.isStreaming).toBe(false);
		}
	});
});

describe('applyEvent — tool calls', () => {
	it('tool_call_recorded adds a completed tool card to the current agent row', () => {
		const state = makeTranscriptState();
		applyEvent(state, evt('run_started', { objective: 'Task' }));
		applyEvent(state, evt('turn_delta', { text: '' }, 'evt-2'));
		applyEvent(
			state,
			evt(
				'tool_call_recorded',
				{
					tool_call_id: 'tc-1',
					tool_name: 'read_file',
					input_json: { path: '/tmp/foo' },
					output_json: { content: 'hello' }
				},
				'evt-3'
			)
		);
		const agentRow = state.rows[1];
		if (agentRow.role === 'agent') {
			expect(agentRow.toolCalls).toHaveLength(1);
			expect(agentRow.toolCalls[0].name).toBe('read_file');
			expect(agentRow.toolCalls[0].status).toBe('completed');
			expect(agentRow.toolCalls[0].expanded).toBe(false);
		}
	});

	it('tool_log_recorded appends log to the matching tool call', () => {
		const state = makeTranscriptState();
		applyEvent(state, evt('run_started', { objective: 'Task' }));
		applyEvent(state, evt('turn_delta', { text: '' }, 'evt-2'));
		applyEvent(
			state,
			evt(
				'tool_log_recorded',
				{ tool_call_id: 'tc-1', tool_name: 'bash', text: 'log line 1' },
				'evt-3'
			)
		);
		applyEvent(
			state,
			evt(
				'tool_call_recorded',
				{ tool_call_id: 'tc-1', tool_name: 'bash', input_json: {}, output_json: {} },
				'evt-4'
			)
		);
		const agentRow = state.rows[1];
		if (agentRow.role === 'agent') {
			const toolCall = agentRow.toolCalls.find((t) => t.id === 'tc-1');
			expect(toolCall?.logs).toContain('log line 1');
		}
	});
});

describe('applyEvent — session_message_added', () => {
	it('inbound session message creates a user row', () => {
		const state = makeTranscriptState();
		applyEvent(
			state,
			evt('session_message_added', { kind: 'inbound', body: 'Hi from user' }, 'evt-1')
		);
		expect(state.rows).toHaveLength(1);
		expect(state.rows[0].role).toBe('user');
		expect(state.rows[0].text).toBe('Hi from user');
	});

	it('outbound session message creates an agent row', () => {
		const state = makeTranscriptState();
		applyEvent(
			state,
			evt('session_message_added', { kind: 'outbound', body: 'Reply from agent' }, 'evt-1')
		);
		expect(state.rows).toHaveLength(1);
		expect(state.rows[0].role).toBe('agent');
		expect(state.rows[0].text).toBe('Reply from agent');
	});
});

describe('applyEvent — unknown events', () => {
	it('ignores events with unknown kinds', () => {
		const state = makeTranscriptState();
		applyEvent(state, evt('unknown_event_kind', {}));
		expect(state.rows).toHaveLength(0);
		expect(state.runStatus).toBe('idle');
	});
});
