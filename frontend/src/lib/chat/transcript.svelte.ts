import type { ReplayDeltaEnvelope } from '$lib/http/events';
import type {
	RunCompletedPayload,
	RunStartedPayload,
	SessionMessageAddedPayload,
	ToolCallRecordedPayload,
	ToolLogRecordedPayload,
	TranscriptRow,
	TurnCompletedPayload,
	TurnDeltaPayload
} from './types';

export interface TokenSummary {
	inputTokens: number;
	outputTokens: number;
}

export type RunStatus = 'idle' | 'active' | 'completed' | 'failed' | 'interrupted';

export class TranscriptState {
	rows = $state<TranscriptRow[]>([]);
	runStatus = $state<RunStatus>('idle');
	activeRunId = $state<string | null>(null);
	tokenSummary = $state<TokenSummary>({ inputTokens: 0, outputTokens: 0 });

	reset(): void {
		this.rows = [];
		this.runStatus = 'idle';
		this.activeRunId = null;
		this.tokenSummary = { inputTokens: 0, outputTokens: 0 };
	}
}

export function makeTranscriptState(): TranscriptState {
	return new TranscriptState();
}

// Returns the current agent row if it is still streaming, otherwise null.
function activeAgentRow(state: TranscriptState): Extract<TranscriptRow, { role: 'agent' }> | null {
	if (state.rows.length === 0) return null;
	const last = state.rows[state.rows.length - 1];
	if (last.role === 'agent' && last.isStreaming) return last;
	return null;
}

// Returns the most recent agent row regardless of streaming state.
function lastAgentRow(state: TranscriptState): Extract<TranscriptRow, { role: 'agent' }> | null {
	for (let i = state.rows.length - 1; i >= 0; i--) {
		if (state.rows[i].role === 'agent') {
			return state.rows[i] as Extract<TranscriptRow, { role: 'agent' }>;
		}
	}
	return null;
}

function ensureStreamingAgentRow(
	state: TranscriptState,
	eventId: string,
	timestamp: string
): Extract<TranscriptRow, { role: 'agent' }> {
	const existing = activeAgentRow(state);
	if (existing) return existing;
	const row: Extract<TranscriptRow, { role: 'agent' }> = {
		id: eventId,
		role: 'agent',
		text: '',
		isStreaming: true,
		toolCalls: [],
		timestamp
	};
	state.rows.push(row);
	return row;
}

export function applyEvent(state: TranscriptState, delta: ReplayDeltaEnvelope): void {
	const id = delta.event_id ?? `${delta.run_id}-${delta.kind}-${delta.occurred_at}`;
	const timestamp = delta.occurred_at;
	const payload = delta.payload as Record<string, unknown> | null | undefined;

	switch (delta.kind) {
		case 'run_started': {
			const p = payload as RunStartedPayload | null;
			const objective = p?.objective ?? '';
			state.rows.push({ id, role: 'user', text: objective, timestamp });
			state.runStatus = 'active';
			state.activeRunId = delta.run_id;
			break;
		}

		case 'turn_delta': {
			const p = payload as TurnDeltaPayload | null;
			const text = p?.text ?? '';
			if (!text) break;
			const row = ensureStreamingAgentRow(state, id, timestamp);
			row.text += text;
			break;
		}

		case 'turn_completed': {
			const p = payload as TurnCompletedPayload | null;
			const content = p?.content ?? '';
			const existing = activeAgentRow(state);
			if (existing) {
				existing.text = content || existing.text;
				existing.isStreaming = false;
			} else {
				state.rows.push({
					id,
					role: 'agent',
					text: content,
					isStreaming: false,
					toolCalls: [],
					timestamp
				});
			}
			break;
		}

		case 'tool_call_recorded': {
			const p = payload as ToolCallRecordedPayload | null;
			if (!p?.tool_call_id) break;
			let agentRow = lastAgentRow(state);
			if (!agentRow) {
				agentRow = ensureStreamingAgentRow(state, id, timestamp);
			}
			const existing = agentRow.toolCalls.find((t) => t.id === p.tool_call_id);
			if (existing) {
				existing.outputJSON = p.output_json != null ? JSON.stringify(p.output_json) : undefined;
				existing.status = 'completed';
			} else {
				agentRow.toolCalls.push({
					id: p.tool_call_id,
					name: p.tool_name,
					inputJSON: p.input_json != null ? JSON.stringify(p.input_json) : '',
					outputJSON: p.output_json != null ? JSON.stringify(p.output_json) : undefined,
					logs: [],
					status: 'completed',
					expanded: false
				});
			}
			break;
		}

		case 'tool_log_recorded': {
			const p = payload as ToolLogRecordedPayload | null;
			const toolCallId = p?.tool_call_id;
			const logText = p?.text ?? p?.body ?? '';
			if (!toolCallId || !logText) break;
			let agentRow = lastAgentRow(state);
			if (!agentRow) {
				agentRow = ensureStreamingAgentRow(state, id, timestamp);
			}
			let toolCall = agentRow.toolCalls.find((t) => t.id === toolCallId);
			if (!toolCall) {
				toolCall = {
					id: toolCallId,
					name: p?.tool_name ?? toolCallId,
					inputJSON: '',
					logs: [],
					status: 'active',
					expanded: false
				};
				agentRow.toolCalls.push(toolCall);
			}
			toolCall.logs.push(logText);
			break;
		}

		case 'run_completed': {
			const p = payload as RunCompletedPayload | null;
			state.runStatus = 'completed';
			state.tokenSummary = {
				inputTokens: p?.input_tokens ?? 0,
				outputTokens: p?.output_tokens ?? 0
			};
			const streaming = activeAgentRow(state);
			if (streaming) streaming.isStreaming = false;
			break;
		}

		case 'run_interrupted':
			state.runStatus = 'interrupted';
			break;

		case 'run_failed':
			state.runStatus = 'failed';
			break;

		case 'session_message_added': {
			const p = payload as SessionMessageAddedPayload | null;
			const body = p?.body ?? '';
			const kind = p?.kind ?? '';
			if (kind === 'steer' || kind === 'announce' || kind === 'spawn') {
				state.rows.push({ id, role: 'system', text: body, timestamp });
			} else if (kind === 'assistant' || kind === 'agent_send' || kind === 'outbound') {
				state.rows.push({
					id,
					role: 'agent',
					text: body,
					isStreaming: false,
					toolCalls: [],
					timestamp
				});
			} else {
				state.rows.push({ id, role: 'user', text: body, timestamp });
			}
			break;
		}

		case 'run_resumed':
		case 'approval_requested':
		case 'run_updated':
			break;

		default:
			break;
	}
}
