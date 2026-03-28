<script lang="ts">
	import { resolve } from '$app/paths';
	import type { WorkClusterResponse } from '$lib/types/api';

	let {
		cluster,
		actionLabel = 'Open run graph'
	}: {
		cluster: WorkClusterResponse;
		actionLabel?: string;
	} = $props();
</script>

<article class="gc-panel-soft px-4 py-4">
	<div class="flex items-start justify-between gap-4">
		<div>
			<p class="gc-stamp">{cluster.root.agent_id}</p>
			<h3 class="gc-panel-title mt-3 text-[1rem]">{cluster.root.objective}</h3>
		</div>
		<p class="gc-machine">{cluster.root.status_label}</p>
	</div>

	<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">{cluster.blocker_label}</p>

	<div class="mt-4 grid gap-2 md:grid-cols-2">
		<div class="border-t-2 border-[var(--gc-border)] pt-3">
			<p class="gc-stamp">Model</p>
			<p class="gc-machine mt-2">{cluster.root.model_display}</p>
		</div>
		<div class="border-t-2 border-[var(--gc-border)] pt-3">
			<p class="gc-stamp">Children</p>
			<p class="gc-machine mt-2">{cluster.child_count_label}</p>
		</div>
	</div>

	{#if cluster.children.length > 0}
		<div class="mt-5 grid gap-2 border-t-2 border-[var(--gc-border)] pt-4">
			{#each cluster.children as child (child.id)}
				<div class="flex items-center justify-between gap-3">
					<div>
						<p class="gc-stamp">{child.agent_id}</p>
						<p class="gc-copy mt-1 text-[var(--gc-ink)]">{child.objective}</p>
					</div>
					<p class="gc-machine">{child.status_label}</p>
				</div>
			{/each}
		</div>
	{/if}

	<a
		href={resolve('/work/[runId]', { runId: cluster.root.id })}
		class="mt-5 inline-flex border-2 border-[var(--gc-cyan)] px-4 py-3 text-sm font-[var(--gc-font-mono)] font-bold tracking-[0.18em] uppercase transition-colors hover:bg-[rgba(83,199,240,0.1)]"
	>
		{actionLabel}
	</a>
</article>
