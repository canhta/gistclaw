import { loadApprovalPolicy } from '$lib/approvals/load';
import { loadRecover } from '$lib/recover/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const [recover, policy] = await Promise.allSettled([
		loadRecover(fetch, 'status=pending'),
		loadApprovalPolicy(fetch)
	]);

	return {
		approvals: {
			items: recover.status === 'fulfilled' ? (recover.value.approvals ?? []) : [],
			paging:
				recover.status === 'fulfilled'
					? recover.value.approval_paging
					: { has_next: false, has_prev: false },
			openCount: recover.status === 'fulfilled' ? (recover.value.summary?.open_approvals ?? 0) : 0,
			summary: {
				pendingCount:
					recover.status === 'fulfilled' ? (recover.value.summary?.pending_approvals ?? 0) : 0,
				connectorCount:
					recover.status === 'fulfilled' ? (recover.value.summary?.connector_count ?? 0) : 0,
				activeRoutes:
					recover.status === 'fulfilled' ? (recover.value.summary?.active_routes ?? 0) : 0
			},
			policy: policy.status === 'fulfilled' ? policy.value : null
		}
	};
};
