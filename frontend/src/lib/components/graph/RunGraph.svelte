<script lang="ts">
	import { Background, BackgroundVariant, Controls, MiniMap, SvelteFlow } from '@xyflow/svelte';
	import '@xyflow/svelte/dist/style.css';
	import type { WorkGraphNodeResponse, WorkGraphResponse } from '$lib/types/api';
	import { buildFlowGraph } from './layout';
	import RunGraphNode from './RunGraphNode.svelte';

	let {
		graph,
		inspectorSeedID
	}: {
		graph: WorkGraphResponse;
		inspectorSeedID?: string;
	} = $props();

	const flow = $derived(buildFlowGraph(graph));
	const nodeTypes = {
		run: RunGraphNode
	};
	const activePathNodes = $derived(
		graph.active_path
			.map((nodeID) => graph.nodes.find((node) => node.id === nodeID))
			.filter((node): node is WorkGraphNodeResponse => node !== undefined)
	);
</script>

<div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_20rem]">
	<section class="gc-panel overflow-hidden px-4 py-4 lg:px-5 lg:py-5">
		<div
			class="flex flex-wrap items-start justify-between gap-4 border-b-2 border-[var(--gc-border)] pb-4"
		>
			<div>
				<p class="gc-stamp">Graph surface</p>
				<h2 class="gc-panel-title mt-3 text-[1.2rem]">{graph.headline}</h2>
			</div>
			<div class="grid grid-cols-3 gap-2 md:grid-cols-4">
				<div class="gc-panel-soft px-3 py-3">
					<p class="gc-stamp">Nodes</p>
					<p class="gc-value mt-2 text-[1.05rem]">{graph.summary.total}</p>
				</div>
				<div class="gc-panel-soft px-3 py-3">
					<p class="gc-stamp">Active</p>
					<p class="gc-value mt-2 text-[1.05rem]">{graph.summary.active}</p>
				</div>
				<div class="gc-panel-soft px-3 py-3">
					<p class="gc-stamp">Approvals</p>
					<p class="gc-value mt-2 text-[1.05rem]">{graph.summary.needs_approval}</p>
				</div>
				<div class="gc-panel-soft px-3 py-3">
					<p class="gc-stamp">Failed</p>
					<p class="gc-value mt-2 text-[1.05rem]">{graph.summary.failed}</p>
				</div>
			</div>
		</div>

		<div class="mt-4 h-[40rem] border-2 border-[var(--gc-border-strong)] bg-[var(--gc-canvas)]">
			<SvelteFlow
				nodes={flow.nodes}
				edges={flow.edges}
				{nodeTypes}
				fitView
				minZoom={0.2}
				maxZoom={1.6}
				class="h-full w-full"
			>
				<Controls />
				<MiniMap pannable zoomable />
				<Background gap={24} size={1} variant={BackgroundVariant.Cross} />
			</SvelteFlow>
		</div>
	</section>

	<aside class="grid gap-4">
		<div class="gc-panel px-4 py-4">
			<p class="gc-stamp">Active path</p>
			<div class="mt-4 grid gap-3">
				{#each activePathNodes as node (node.id)}
					<div
						data-active-node={node.id}
						class={`gc-panel-soft px-3 py-3 ${node.id === inspectorSeedID ? 'border-[var(--gc-orange)]' : ''}`}
					>
						<p class="gc-stamp">{node.agent_id}</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">
							{node.objective_preview || node.objective}
						</p>
						<p class="gc-machine mt-2">{node.id}</p>
					</div>
				{/each}
			</div>
		</div>

		<div class="gc-panel-soft px-4 py-4">
			<p class="gc-stamp">Run ledger</p>
			<div class="mt-4 grid gap-3">
				{#each graph.nodes as node (node.id)}
					<article
						data-run-ledger={node.id}
						class="border-t-2 border-[var(--gc-border)] pt-3 first:border-t-0 first:pt-0"
					>
						<p class="gc-stamp">{node.agent_id}</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">
							{node.objective_preview || node.objective}
						</p>
						<p class="gc-machine mt-2">{node.status_label} · {node.token_summary}</p>
					</article>
				{/each}
			</div>
		</div>
	</aside>
</div>
