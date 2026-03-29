import type { LogEntryResponse } from '$lib/types/api';

interface LogEventSource {
	onmessage: ((event: MessageEvent<string>) => void) | null;
	onerror: ((event: Event) => void) | null;
	close(): void;
}

type LogEventSourceFactory = (url: string, init: EventSourceInit) => LogEventSource;

export function connectLogStream(
	url: string,
	onMessage: (entry: LogEntryResponse) => void,
	onError: (event: Event) => void = () => {},
	createSource: LogEventSourceFactory = (input, init) => new EventSource(input, init)
): () => void {
	const source = createSource(url, { withCredentials: true });

	source.onmessage = (event) => {
		try {
			onMessage(JSON.parse(event.data) as LogEntryResponse);
		} catch {
			// Ignore malformed payloads and keep the tail alive.
		}
	};

	source.onerror = (event) => {
		onError(event);
	};

	return () => {
		source.close();
	};
}
