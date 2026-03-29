import { requestJSON } from '$lib/http/client';
import type { ApprovalPolicyResponse } from '$lib/types/api';

export function loadApprovalPolicy(fetcher: typeof fetch): Promise<ApprovalPolicyResponse> {
	return requestJSON<ApprovalPolicyResponse>(fetcher, '/api/approvals/policy');
}
