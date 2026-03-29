<script lang="ts">
	import ApprovalRow from '$lib/components/approvals/ApprovalRow.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import { requestJSON } from '$lib/http/client';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = [
		{ id: 'gateway', label: 'Gateway' },
		{ id: 'nodes', label: 'Nodes' },
		{ id: 'allowlists', label: 'Allowlists' }
	];

	let activeTab = $state('gateway');
	let confirmApprovalID = $state<string | null>(null);
	let actionMessage = $state('');
	let actionError = $state('');
	let resolvedApprovalIDs = $state<string[]>([]);

	const approvals = $derived(
		(data.approvals?.items ?? [])
			.filter((item) => item.status === 'pending')
			.filter((item) => !resolvedApprovalIDs.includes(item.id))
	);
	const openCount = $derived(
		Math.max(0, (data.approvals?.openCount ?? approvals.length) - resolvedApprovalIDs.length)
	);
	const confirmApproval = $derived(approvals.find((item) => item.id === confirmApprovalID) ?? null);

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
		<SectionTabs {tabs} bind:activeTab />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
		{#if activeTab === 'gateway'}
			<div
				class="shrink-0 border-b border-[var(--gc-border)] bg-[var(--gc-surface-raised)] px-5 py-4"
			>
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
				<div class="border-b border-[var(--gc-border)] px-5 py-4">
					<SurfaceMessage label="UPDATED" message={actionMessage} />
				</div>
			{/if}

			{#if actionError}
				<div class="border-b border-[var(--gc-border)] px-5 py-4">
					<SurfaceMessage label="ACTION FAILED" message={actionError} tone="error" />
				</div>
			{/if}

			<div class="min-h-0 flex-1 overflow-auto">
				{#if approvals.length === 0}
					<div class="flex h-full items-center justify-center p-10">
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
					{#each approvals as approval (approval.id)}
						<ApprovalRow
							{approval}
							onApprove={requestApprove}
							onDeny={(id) => void denyApproval(id)}
						/>
					{/each}
				{/if}
			</div>
		{:else if activeTab === 'nodes'}
			<div class="flex flex-1 items-center justify-center p-10">
				<div class="text-center">
					<p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
					<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Node policy</p>
					<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
						Per-node exec approval policy will land here once the web API exposes those controls.
					</p>
				</div>
			</div>
		{:else}
			<div class="flex flex-1 items-center justify-center p-10">
				<div class="text-center">
					<p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
					<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Allowlists</p>
					<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
						Path and command allowlists will appear here when the backend exposes editable entries.
					</p>
				</div>
			</div>
		{/if}
	</div>
</div>
