// Transcript row roles
export type TranscriptRole = 'user' | 'agent' | 'system';

// Status of a tool call within an agent row
export type ToolCallStatus = 'active' | 'completed' | 'failed';

// A single tool call card within an agent transcript row
export interface ToolCall {
	id: string; // tool_call_id
	name: string;
	inputJSON: string;
	outputJSON?: string;
	logs: string[]; // accumulated log lines
	status: ToolCallStatus;
	expanded: boolean;
}

// A single row in the transcript timeline
export type TranscriptRow =
	| { id: string; role: 'user'; text: string; timestamp: string }
	| {
			id: string;
			role: 'agent';
			text: string;
			isStreaming: boolean;
			toolCalls: ToolCall[];
			timestamp: string;
	  }
	| { id: string; role: 'system'; text: string; timestamp: string };

// Run status for the composer / inspector
export type RunStatus = 'idle' | 'active' | 'completed' | 'failed' | 'interrupted';

// Payload shapes for SSE events — only what Chat needs
export interface RunStartedPayload {
	objective: string;
}

export interface TurnDeltaPayload {
	text: string;
}

export interface TurnCompletedPayload {
	content: string;
	input_tokens?: number;
	output_tokens?: number;
}

export interface ToolCallRecordedPayload {
	tool_call_id: string;
	tool_name: string;
	input_json?: unknown;
	output_json?: unknown;
	decision?: string;
}

export interface ToolLogRecordedPayload {
	tool_call_id?: string;
	tool_name?: string;
	text?: string;
	body?: string;
}

export interface RunCompletedPayload {
	input_tokens?: number;
	output_tokens?: number;
}

export interface SessionMessageAddedPayload {
	kind: string; // e.g. "text", "inbound"
	body: string;
	sender_session_id?: string;
}
