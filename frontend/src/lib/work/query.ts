export function buildWorkListSearch(params: URLSearchParams): string {
	const next = new URLSearchParams();

	for (const key of ['cursor', 'direction', 'limit']) {
		const value = params.get(key);
		if (value != null && value.trim() !== '') {
			next.set(key, value);
		}
	}

	return next.toString();
}

export function buildChatPageHref(
	apiHref: string | undefined,
	currentSearch = ''
): string | undefined {
	if (apiHref == null || apiHref.trim() === '') {
		return undefined;
	}

	const url = new URL(apiHref, 'http://localhost');
	const next = new URLSearchParams(url.search);
	const current = new URLSearchParams(currentSearch);

	for (const key of ['tab', 'run']) {
		const value = current.get(key);
		if (value != null && value.trim() !== '') {
			next.set(key, value);
		}
	}

	const suffix = next.toString();
	return suffix === '' ? '/chat' : `/chat?${suffix}`;
}
