<script lang="ts">
	import type { RecoverApprovalResponse } from '$lib/types/api';

	let {
		approval,
		onApprove,
		onDeny
	}: {
		approval: RecoverApprovalResponse;
		onApprove?: (id: string) => void;
		onDeny?: (id: string) => void;
	} = $props();

	const isPending = $derived(approval.status === 'pending');

	const statusToneByClass: Record<string, string> = {
		'is-success': 'var(--gc-success)',
		'is-error': 'var(--gc-error)',
		'is-active': 'var(--gc-warning)',
		'is-muted': 'var(--gc-ink-3)'
	};

	const statusTone = $derived(statusToneByClass[approval.status_class] ?? 'var(--gc-ink-3)');
</script>

<div class="border-b border-[var(--gc-border)] px-5 py-4">
	<div class="flex items-start justify-between gap-4">
		<div class="min-w-0 flex-1">
			<div class="flex flex-wrap items-center gap-3">
				<span class="gc-stamp text-[var(--gc-ink-2)]">{approval.tool_name}</span>
				<span class="gc-badge" style={`border-color: ${statusTone}; color: ${statusTone};`}>
					{approval.status_label}
				</span>
			</div>
			<p class="gc-copy mt-2 font-mono break-all text-[var(--gc-signal)]">
				{approval.binding_summary}
			</p>
			<div class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1">
				<span class="gc-machine text-[var(--gc-ink-4)]">run {approval.run_id}</span>
				{#if approval.resolved_at_label}
					<span class="gc-machine text-[var(--gc-ink-4)]"
						>resolved {approval.resolved_at_label}</span
					>
				{/if}
				{#if approval.resolved_by}
					<span class="gc-machine text-[var(--gc-ink-4)]">by {approval.resolved_by}</span>
				{/if}
			</div>
		</div>

		{#if isPending}
			<div class="flex shrink-0 gap-2">
				<button
					type="button"
					onclick={() => onDeny?.(approval.id)}
					class="gc-action px-3 py-1 text-[var(--gc-error)]"
				>
					DENY
				</button>
				<button
					type="button"
					onclick={() => onApprove?.(approval.id)}
					class="gc-action gc-action-warning px-3 py-1"
				>
					APPROVE
				</button>
			</div>
		{/if}
	</div>
</div>
