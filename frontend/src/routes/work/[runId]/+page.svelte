<script lang="ts">
	import { resolve } from '$app/paths';
	import RunGraph from '$lib/components/graph/RunGraph.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	const detail = $derived(data.work);
</script>

<svelte:head>
	<title>{detail.run.objective_text} | Work | GistClaw</title>
</svelte:head>

<div class="grid gap-6">
	<section class="grid gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<p class="gc-stamp">Run detail</p>
			<h2 class="gc-section-title mt-3">{detail.run.objective_text}</h2>
			<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">{detail.run.state_label}</p>

			<div class="mt-6 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
				<div class="gc-panel-soft px-4 py-4">
					<p class="gc-stamp">Status</p>
					<p class="gc-value mt-3 text-[1.05rem]">{detail.run.status_label}</p>
				</div>
				<div class="gc-panel-soft px-4 py-4">
					<p class="gc-stamp">Trigger</p>
					<p class="gc-value mt-3 text-[1.05rem]">{detail.run.trigger_label}</p>
				</div>
				<div class="gc-panel-soft px-4 py-4">
					<p class="gc-stamp">Model</p>
					<p class="gc-value mt-3 text-[1.05rem]">{detail.run.model_display}</p>
				</div>
				<div class="gc-panel-soft px-4 py-4">
					<p class="gc-stamp">Tokens</p>
					<p class="gc-value mt-3 text-[1.05rem]">{detail.run.token_summary}</p>
				</div>
			</div>
		</div>

		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<p class="gc-stamp">Inspector seed</p>
			<h2 class="gc-section-title mt-3">
				{detail.inspector_seed?.agent_id ?? 'No focused node selected yet'}
			</h2>
			<p class="gc-machine mt-4">
				{detail.inspector_seed?.id ?? detail.run.id} · {detail.inspector_seed?.status ??
					detail.run.status}
			</p>

			<a
				href={resolve('/work')}
				class="mt-6 inline-flex border-2 border-[var(--gc-border-strong)] px-4 py-3 text-sm font-[var(--gc-font-mono)] font-bold tracking-[0.18em] uppercase transition-colors hover:border-[var(--gc-cyan)]"
			>
				Back to queue
			</a>
		</div>
	</section>

	<RunGraph graph={detail.graph} inspectorSeedID={detail.inspector_seed?.id} />
</div>
