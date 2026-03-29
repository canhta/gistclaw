export function defaultAnchorAt(now = new Date()): string {
	const local = new Date(now);
	local.setSeconds(0, 0);
	local.setMinutes(local.getMinutes() - local.getTimezoneOffset());
	return local.toISOString().slice(0, 16);
}

export function serializeAnchorAt(raw: string): string {
	if (raw.trim() === '') {
		return '';
	}
	return new Date(raw).toISOString();
}
