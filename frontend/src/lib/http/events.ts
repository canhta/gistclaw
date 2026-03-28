export interface ReplayDeltaEnvelope {
	event_id?: string;
	run_id: string;
	kind: string;
	payload?: unknown;
	occurred_at: string;
}

export interface EventStreamSource {
	onmessage: ((event: MessageEvent<string>) => void) | null;
	onerror: ((event: Event) => void) | null;
	close(): void;
}

export type EventStreamFactory = (url: string, init: EventSourceInit) => EventStreamSource;

export function connectEventStream(
	url: string,
	onMessage: (event: ReplayDeltaEnvelope) => void,
	onError: (event: Event) => void = () => {},
	createSource: EventStreamFactory = (input, init) => new EventSource(input, init)
): () => void {
	const source = createSource(url, { withCredentials: true });

	source.onmessage = (event) => {
		try {
			onMessage(JSON.parse(event.data) as ReplayDeltaEnvelope);
		} catch {
			// Ignore malformed events and keep the stream alive.
		}
	};

	source.onerror = (event) => {
		onError(event);
	};

	return () => {
		source.close();
	};
}
