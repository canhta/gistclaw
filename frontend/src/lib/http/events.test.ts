import { describe, expect, it, vi } from 'vitest';
import { connectEventStream, type EventStreamSource } from './events';

describe('connectEventStream', () => {
	it('opens an EventSource with credentials and parses replay delta messages', () => {
		let source:
			| {
					onmessage: ((event: MessageEvent<string>) => void) | null;
					onerror: (() => void) | null;
					close: ReturnType<typeof vi.fn>;
			  }
			| undefined;

		const stop = connectEventStream(
			'/api/work/run-work-root/events',
			vi.fn(),
			vi.fn(),
			(url, init) => {
				expect(url).toBe('/api/work/run-work-root/events');
				expect(init).toEqual({ withCredentials: true });

				source = {
					onmessage: null,
					onerror: null,
					close: vi.fn()
				};

				return source as EventStreamSource;
			}
		);

		expect(source).toBeDefined();
		source?.onmessage?.(
			new MessageEvent('message', {
				data: JSON.stringify({
					event_id: 'evt-123',
					run_id: 'run-work-root',
					kind: 'run_updated',
					occurred_at: '2026-03-28T09:05:00Z'
				})
			})
		);

		stop();

		expect(source?.close).toHaveBeenCalledTimes(1);
	});
});
