export class HTTPError extends Error {
	readonly status: number;

	constructor(status: number, message: string) {
		super(message);
		this.name = 'HTTPError';
		this.status = status;
	}
}

export async function requestJSON<T>(
	fetcher: typeof fetch,
	input: RequestInfo | URL,
	init?: RequestInit
): Promise<T> {
	const response = await fetcher(input, {
		...init,
		headers: {
			accept: 'application/json',
			...init?.headers
		}
	});

	const contentType = response.headers.get('content-type') ?? '';
	const isJSON = contentType.includes('application/json');
	const payload = isJSON ? await response.json() : await response.text();

	if (!response.ok) {
		const message =
			payload &&
			typeof payload === 'object' &&
			'message' in payload &&
			typeof payload.message === 'string'
				? payload.message
				: typeof payload === 'string' && payload.trim() !== ''
					? payload
					: response.statusText || 'Request failed';

		throw new HTTPError(response.status, message);
	}

	if (!isJSON) {
		throw new Error(`Expected JSON response for ${String(input)}`);
	}

	return payload as T;
}
