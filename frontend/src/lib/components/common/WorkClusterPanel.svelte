<script lang="ts">
	import { resolve } from '$app/paths';
	import type { WorkClusterResponse } from '$lib/types/api';

	let {
		cluster
	}: {
		cluster: WorkClusterResponse;
	} = $props();

	function formatAgentID(agentID: string): string {
		return agentID.replace(/_/g, ' ').replace(/\b\w/g, (c, i) => (i === 0 ? c.toUpperCase() : c));
	}

	function toneClass(statusClass: string): string {
		if (statusClass.includes('approval')) return 'border-[var(--gc-orange)]';
		if (
			statusClass.includes('error') ||
			statusClass.includes('failed') ||
			statusClass.includes('interrupted')
		) {
			return 'border-[var(--gc-error)]';
		}
		if (statusClass.includes('active')) return 'border-[var(--gc-cyan)]';
		return 'border-[var(--gc-border-strong)]';
	}

	function statusTextClass(statusClass: string): string {
		if (statusClass.includes('approval')) return 'text-[var(--gc-orange)]';
		if (
			statusClass.includes('error') ||
			statusClass.includes('failed') ||
			statusClass.includes('interrupted')
		) {
			return 'text-[var(--gc-error)]';
		}
		if (statusClass.includes('active')) return 'text-[var(--gc-cyan)]';
		return 'text-[var(--gc-text-secondary)]';
	}

	const panelTone = $derived(toneClass(cluster.root.status_class));
</script>

<article class={`gc-panel-soft border-2 px-4 py-4 ${panelTone}`}>
	<!-- Root agent -->
	<div class="flex items-start justify-between gap-4">
		<div class="min-w-0">
			<p class="gc-stamp">{formatAgentID(cluster.root.agent_id)}</p>
			<h3 class="gc-panel-title mt-2 text-[1rem]">{cluster.root.objective}</h3>
		</div>
		<p class={`gc-stamp shrink-0 ${statusTextClass(cluster.root.status_class)}`}>
			{cluster.root.status_label}
		</p>
	</div>

	<!-- Worker agents -->
	{#if cluster.children.length > 0}
		<div class="mt-4 border-l-2 border-[var(--gc-border)] pl-4">
			<div class="grid gap-3">
				{#each cluster.children as child (child.id)}
					<div class={`gc-panel-soft border-l-2 px-3 py-3 ${toneClass(child.status_class)}`}>
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0">
								<p class="gc-stamp">{formatAgentID(child.agent_id)}</p>
								<p class="gc-copy mt-1 truncate text-[var(--gc-ink)]">{child.objective}</p>
							</div>
							<div class="shrink-0 text-right">
								<p class={`gc-stamp ${statusTextClass(child.status_class)}`}>
									{child.status_label}
								</p>
								<p class="gc-machine mt-1">{child.last_activity_short}</p>
							</div>
						</div>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Footer -->
	<div
		class="mt-4 flex items-center justify-between gap-4 border-t-2 border-[var(--gc-border)] pt-3"
	>
		<div class="flex gap-4">
			<div>
				<p class="gc-stamp">Started</p>
				<p class="gc-machine mt-1">{cluster.root.started_at_short}</p>
			</div>
			{#if cluster.has_children}
				<div>
					<p class="gc-stamp">Workers</p>
					<p class="gc-machine mt-1">{cluster.child_count_label}</p>
				</div>
			{/if}
		</div>
		<a
			href={resolve('/work/[runId]', { runId: cluster.root.id })}
			class="gc-action gc-action-accent px-4 py-2"
		>
			Open graph
		</a>
	</div>
</article>
