import { loadRecover } from '$lib/recover/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	try {
		const data = await loadRecover(fetch, 'status=pending');
		return {
			approvals: {
				items: data.approvals ?? [],
				paging: data.approval_paging,
				openCount: data.summary?.open_approvals ?? 0
			}
		};
	} catch {
		return {
			approvals: {
				items: [],
				paging: { has_next: false, has_prev: false },
				openCount: 0
			}
		};
	}
};
