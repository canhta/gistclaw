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
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
							Node approval policy remains centralized at the gateway.
						</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							GistClaw already runs worker sessions under the same runtime approval wall, but the
							web API does not expose per-node policy editing yet. Keep worker sessions on the
							shared gateway defaults until runtime policy moves into a dedicated node control
							surface.
						</p>
						<div class="mt-5">
							<SurfaceMessage
								label="DEFERRED CONTROL"
								message="Use the gateway queue for live decisions today; per-node policy still belongs to runtime and config seams."
								tone="error"
							/>
						</div>
					</div>

					<div class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Current operator path</p>
						<div class="mt-4 space-y-4">
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Sessions</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Worker sessions show which agent asked for host access and where the request came
									from.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Debug</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Check runtime state and connector health before widening policy for a noisy node.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Gateway queue</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Review each pending command directly until node-scoped approval controls exist.
								</p>
							</div>
						</div>
					</div>
				</div>
			{:else}
				<div class="grid flex-1 gap-5 xl:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]">
					<div class="gc-panel px-5 py-5">
						<p class="gc-stamp text-[var(--gc-warning)]">ALLOWLISTS</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
							Allowlists are still managed outside the browser.
						</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							Path and command allowlists have not been promoted into a browser editor yet. Treat
							the Gateway queue as the active safety boundary and only broaden long-lived exceptions
							after you confirm the command pattern in project config and live run evidence.
						</p>
						<div class="mt-5">
							<SurfaceMessage
								label="MANUAL SEAM"
								message="Keep broad exec exceptions in Config or runtime-managed policy until the recover API exposes editable allowlists."
							/>
						</div>
					</div>

					<div class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Verification trail</p>
						<div class="mt-4 space-y-4">
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Config</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Confirm the project and runtime settings before adding any durable exception.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Gateway queue</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Use one-off approvals while you decide whether a repeat command deserves an
									allowlist entry.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Chat</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Check run context and tool intent in Chat before converting a repeated request
									into a durable exception.
								</p>
							</div>
						</div>
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>
