<script lang="ts">
	import { Handle, Position } from '@xyflow/svelte';
	import type { FlowRunNodeData } from './layout';

	let {
		data,
		selected = false
	}: {
		data: FlowRunNodeData;
		selected?: boolean;
	} = $props();

	function toneClass(statusClass: string): string {
		if (statusClass.includes('approval')) {
			return 'border-[var(--gc-orange)]';
		}
		if (statusClass.includes('active')) {
			return 'border-[var(--gc-cyan)]';
		}
		if (statusClass.includes('failed') || statusClass.includes('interrupted')) {
			return 'border-[var(--gc-error)]';
		}
		return 'border-[var(--gc-border-strong)]';
	}
</script>

<Handle type="target" position={Position.Left} class="!h-3 !w-3 !border-0 !bg-[var(--gc-border)]" />

<div
	data-run-node={data.runID}
	class={`min-w-[15rem] border-2 bg-[var(--gc-surface)] px-4 py-4 text-[var(--gc-ink)] shadow-[0_0_0_1px_rgba(0,0,0,0.32)] ${toneClass(data.statusClass)} ${selected ? 'ring-2 ring-[var(--gc-cyan)]' : ''}`}
>
	<div class="flex items-start justify-between gap-3">
		<div>
			<p class="gc-stamp">{data.agentID}</p>
			<p class="gc-panel-title mt-2 text-[1rem]">{data.objective}</p>
		</div>
		<p class="gc-machine text-right">{data.shortID}</p>
	</div>

	<div class="mt-4 grid gap-2">
		<div class="flex items-center justify-between gap-3">
			<p class="gc-stamp">State</p>
			<p class="gc-machine text-[var(--gc-ink)]">{data.statusLabel}</p>
		</div>
		<div class="flex items-center justify-between gap-3">
			<p class="gc-stamp">Model</p>
			<p class="gc-machine">{data.modelDisplay}</p>
		</div>
		<div class="flex items-center justify-between gap-3">
			<p class="gc-stamp">Tokens</p>
			<p class="gc-machine">{data.tokenSummary}</p>
		</div>
	</div>

	{#if data.isActivePath}
		<div class="mt-4 border-t-2 border-[var(--gc-border)] pt-3">
			<p class="gc-stamp text-[var(--gc-cyan)]">Active path</p>
		</div>
	{/if}
</div>

<Handle
	type="source"
	position={Position.Right}
	class="!h-3 !w-3 !border-0 !bg-[var(--gc-border)]"
/>
