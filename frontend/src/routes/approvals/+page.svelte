<script lang="ts">
	import ApprovalRow from '$lib/components/approvals/ApprovalRow.svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import { requestJSON } from '$lib/http/client';
	import type { PageData } from './$types';

	type TabID = 'gateway' | 'nodes' | 'allowlists';

	let { data }: { data: PageData } = $props();

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'gateway', label: 'Gateway' },
		{ id: 'nodes', label: 'Nodes' },
		{ id: 'allowlists', label: 'Allowlists' }
	];

	let activeTabOverride = $state<TabID | null>(null);
	let confirmApprovalID = $state<string | null>(null);
	let actionMessage = $state('');
	let actionError = $state('');
	let resolvedApprovalIDs = $state<string[]>([]);

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'gateway';
	});
	const approvals = $derived(
		(data.approvals?.items ?? [])
			.filter((item) => item.status === 'pending')
			.filter((item) => !resolvedApprovalIDs.includes(item.id))
	);
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const openCount = $derived(
		Math.max(0, (data.approvals?.openCount ?? approvals.length) - resolvedApprovalIDs.length)
	);
	const summary = $derived(
		data.approvals?.summary ?? {
			pendingCount: approvals.length,
			connectorCount: 0,
			activeRoutes: 0
		}
	);
	const policy = $derived(data.approvals?.policy ?? null);
	const policySummary = $derived(
		policy?.summary ?? {
			node_count: 0,
			allowlist_count: 0,
			pending_agents: 0,
			override_agents: 0
		}
	);
	const gatewayPolicy = $derived(
		policy?.gateway ?? {
			approval_mode_label: 'Prompt',
			host_access_mode_label: 'Standard',
			team_name: 'No team loaded',
			front_agent_id: ''
		}
	);
	const policyNodes = $derived(policy?.nodes ?? []);
	const allowlistEntries = $derived(policy?.allowlists ?? []);
	const confirmApproval = $derived(approvals.find((item) => item.id === confirmApprovalID) ?? null);

	function isTabID(value: string | null): value is TabID {
		return value === 'gateway' || value === 'nodes' || value === 'allowlists';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	function requestApprove(id: string): void {
		confirmApprovalID = id;
		actionMessage = '';
		actionError = '';
	}

	function cancelApprove(): void {
		confirmApprovalID = null;
	}

	function listOrNone(values: string[]): string {
		return values.length === 0 ? 'None' : values.join(', ');
	}

	async function resolveApproval(id: string, action: 'approve' | 'deny'): Promise<void> {
		actionMessage = '';
		actionError = '';

		try {
			await requestJSON(globalThis.fetch.bind(globalThis), `/api/recover/approvals/${id}/resolve`, {
				method: 'POST',
				headers: {
					'content-type': 'application/x-www-form-urlencoded;charset=UTF-8'
				},
				body: new URLSearchParams({
					decision: action === 'approve' ? 'approved' : 'denied'
				})
			});

			resolvedApprovalIDs = [...resolvedApprovalIDs, id];

			actionMessage =
				action === 'approve'
					? 'Approval granted. The run can continue.'
					: 'Request denied. The run will be interrupted.';
		} catch {
			actionError =
				action === 'approve'
					? 'Failed to approve the request. Please try again.'
					: 'Failed to deny the request. Please try again.';
		}
	}

	async function confirmApprove(): Promise<void> {
		if (!confirmApprovalID) {
			return;
		}

		const approvalID = confirmApprovalID;
		confirmApprovalID = null;
		await resolveApproval(approvalID, 'approve');
	}

	async function denyApproval(id: string): Promise<void> {
		await resolveApproval(id, 'deny');
	}
</script>

<svelte:head>
	<title>Exec Approvals | GistClaw</title>
</svelte:head>

{#if confirmApprovalID}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-[color-mix(in_srgb,var(--gc-canvas)_84%,transparent)] px-4"
		role="dialog"
		aria-modal="true"
		aria-label="Confirm approval"
	>
		<div class="gc-panel max-w-md px-6 py-5">
			<p class="gc-stamp text-[var(--gc-warning)]">CONFIRM APPROVE</p>
			<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Allow this execution?</h2>
			{#if confirmApproval}
				<div class="gc-panel-soft mt-4 px-4 py-4">
					<div class="flex flex-wrap items-center gap-3">
						<span class="gc-stamp text-[var(--gc-ink-2)]">{confirmApproval.tool_name}</span>
						<span class="gc-badge border-[var(--gc-warning)] text-[var(--gc-warning)]">PENDING</span
						>
					</div>
					<p class="gc-copy mt-3 font-mono break-all text-[var(--gc-signal)]">
						{confirmApproval.binding_summary}
					</p>
					<p class="gc-machine mt-3 text-[var(--gc-ink-4)]">run {confirmApproval.run_id}</p>
				</div>
			{/if}
			<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
				The agent will be allowed to execute this request immediately. Use deny if the command looks
				unsafe or unexpected.
			</p>
			<div class="mt-5 flex justify-end gap-3">
				<button
					type="button"
					onclick={cancelApprove}
					class="gc-action px-4 py-2 text-[var(--gc-ink-2)]"
				>
					Cancel
				</button>
				<button
					type="button"
					onclick={() => void confirmApprove()}
					class="gc-action gc-action-warning px-4 py-2"
				>
					Approve
				</button>
			</div>
		</div>
	</div>
{/if}

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Exec Approvals</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-hidden px-6 py-6">
		<div class="grid gap-4 xl:grid-cols-4">
			<SurfaceMetricCard
				label="Open Queue"
				value={String(openCount)}
				detail="Pending approvals still waiting for an operator decision."
				tone="warning"
			/>
			<SurfaceMetricCard
				label="Routes Holding"
				value={String(summary.pendingCount ?? 0)}
				detail="Runs currently paused behind the gateway approval wall."
			/>
			<SurfaceMetricCard
				label="Connected Lanes"
				value={String(summary.connectorCount ?? 0)}
				detail="Connector lanes currently participating in recovery and approvals."
				tone="accent"
			/>
			<SurfaceMetricCard
				label="Project"
				value={data.project?.active_name ?? 'No active project'}
				detail={data.project?.active_path ?? 'Select a project to scope approvals and recovery.'}
			/>
		</div>

		<div class="mt-5 flex min-h-0 flex-1 overflow-hidden">
			{#if activeTab === 'gateway'}
				<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
					<div class="gc-panel-soft shrink-0 px-5 py-4">
						<div class="flex items-end justify-between gap-4">
							<div>
								<p class="gc-stamp text-[var(--gc-warning)]">GATEWAY QUEUE</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Pending exec approvals pause the run until an operator decides.
								</p>
							</div>
							<div class="text-right">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Open</p>
								<p class="gc-panel-title mt-1 text-[var(--gc-ink)]">{openCount}</p>
							</div>
						</div>
					</div>

					{#if actionMessage}
						<div class="mt-4">
							<SurfaceMessage label="UPDATED" message={actionMessage} />
						</div>
					{/if}

					{#if actionError}
						<div class="mt-4">
							<SurfaceMessage label="ACTION FAILED" message={actionError} tone="error" />
						</div>
					{/if}

					<div class="mt-5 min-h-0 flex-1 overflow-auto">
						{#if approvals.length === 0}
							<div class="gc-panel flex h-full items-center justify-center p-10">
								<div class="text-center">
									<p class="gc-stamp text-[var(--gc-ink-3)]">GATEWAY</p>
									<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No pending approvals</p>
									<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
										The gateway is clear. Approval requests appear here when agents need exec
										permission.
									</p>
								</div>
							</div>
						{:else}
							<div
								class="overflow-hidden rounded-[var(--gc-radius)] border border-[var(--gc-border)]"
							>
								{#each approvals as approval (approval.id)}
									<ApprovalRow
										{approval}
										onApprove={requestApprove}
										onDeny={(id) => void denyApproval(id)}
									/>
								{/each}
							</div>
						{/if}
					</div>
				</div>
			{:else if activeTab === 'nodes'}
				<div class="grid flex-1 gap-5 xl:grid-cols-[minmax(0,1.25fr)_minmax(0,0.9fr)]">
					<div class="gc-panel px-5 py-5">
						<p class="gc-stamp text-[var(--gc-warning)]">NODE POLICY</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Observed node approval posture</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							Each node row combines the active team policy with authority observed on recent
							active-project runs. Use it to spot which agents are inheriting the gateway default
							and which ones have already run with broader authority.
						</p>

						{#if policyNodes.length === 0}
							<div class="mt-5">
								<SurfaceMessage
									label="NO NODES"
									message="No team node policy is available for the active project."
								/>
							</div>
						{:else}
							<div class="mt-5 grid gap-4">
								{#each policyNodes as node (node.agent_id)}
									<div class="border border-[var(--gc-border)] px-4 py-4">
										<div class="flex flex-wrap items-center gap-3">
											<p class="gc-panel-title text-[var(--gc-ink)]">{node.agent_id}</p>
											{#if node.is_front}
												<span class="gc-badge border-[var(--gc-primary)] text-[var(--gc-primary)]">
													FRONT
												</span>
											{/if}
											<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
												{node.base_profile}
											</span>
											{#if node.pending_approvals > 0}
												<span class="gc-badge border-[var(--gc-warning)] text-[var(--gc-warning)]">
													{node.pending_approvals}
													pending
												</span>
											{/if}
										</div>
										<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">{node.role}</p>
										<div class="mt-4 grid gap-3 sm:grid-cols-2">
											<div>
												<p class="gc-stamp text-[var(--gc-ink-3)]">Observed authority</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink)]">
													{node.observed_approval_mode_label} · {node.observed_host_access_mode_label}
												</p>
											</div>
											<div>
												<p class="gc-stamp text-[var(--gc-ink-3)]">Recent runs</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink)]">
													{node.recent_runs} runs · {node.override_runs} overrides
												</p>
											</div>
											<div>
												<p class="gc-stamp text-[var(--gc-ink-3)]">Tool families</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
													{listOrNone(node.tool_families)}
												</p>
											</div>
											<div>
												<p class="gc-stamp text-[var(--gc-ink-3)]">Delegation</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
													{listOrNone(node.delegation_kinds)}
												</p>
											</div>
											<div>
												<p class="gc-stamp text-[var(--gc-ink-3)]">Allow tools</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
													{listOrNone(node.allow_tools)}
												</p>
											</div>
											<div>
												<p class="gc-stamp text-[var(--gc-ink-3)]">Deny tools</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
													{listOrNone(node.deny_tools)}
												</p>
											</div>
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</div>

					<div class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Gateway defaults</p>
						<div class="mt-4 space-y-4">
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">{gatewayPolicy.team_name}</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Front agent {gatewayPolicy.front_agent_id || 'not configured'}.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Approval mode</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									{gatewayPolicy.approval_mode_label}
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Host access</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									{gatewayPolicy.host_access_mode_label}
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Coverage</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									{policySummary.node_count} nodes · {policySummary.pending_agents} with pending approvals
									·
									{` ${policySummary.override_agents}`} with observed overrides
								</p>
							</div>
						</div>
					</div>
				</div>
			{:else}
				<div class="grid flex-1 gap-5 xl:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]">
					<div class="gc-panel px-5 py-5">
						<p class="gc-stamp text-[var(--gc-warning)]">ALLOWLISTS</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Explicit tool allowlists</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							The active team already carries explicit allow and deny lists for individual tools.
							This board shows those durable exceptions directly instead of forcing operators to
							infer them from team files.
						</p>

						{#if allowlistEntries.length === 0}
							<div class="mt-5">
								<SurfaceMessage
									label="NO EXPLICIT ENTRIES"
									message="The active team is relying entirely on the gateway default tool policy."
								/>
							</div>
						{:else}
							<div class="mt-5 grid gap-3">
								{#each allowlistEntries as entry (`${entry.agent_id}:${entry.direction}:${entry.tool_name}`)}
									<div class="border border-[var(--gc-border)] px-4 py-4">
										<div class="flex flex-wrap items-center gap-3">
											<p class="gc-machine text-[var(--gc-ink)]">{entry.tool_name}</p>
											<span
												class={`gc-badge ${entry.direction === 'allow' ? 'border-[var(--gc-success)] text-[var(--gc-success)]' : 'border-[var(--gc-error)] text-[var(--gc-error)]'}`}
											>
												{entry.direction_label}
											</span>
										</div>
										<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
											{entry.agent_id} · {entry.role}
										</p>
									</div>
								{/each}
							</div>
						{/if}
					</div>

					<div class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Current boundary</p>
						<div class="mt-4 space-y-4">
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Gateway default</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									{gatewayPolicy.approval_mode_label} approvals with {gatewayPolicy.host_access_mode_label}
									host access.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Team scope</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									{gatewayPolicy.team_name} · front agent {gatewayPolicy.front_agent_id ||
										'not configured'}.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Explicit entries</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									{policySummary.allowlist_count} durable tool exceptions are active in the current team.
								</p>
							</div>
						</div>
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>
