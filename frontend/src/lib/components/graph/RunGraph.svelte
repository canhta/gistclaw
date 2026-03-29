<script lang="ts">
	import { resolve } from '$app/paths';
	import { Background, BackgroundVariant, Controls, MiniMap, SvelteFlow } from '@xyflow/svelte';
	import '@xyflow/svelte/dist/style.css';
	import type {
		WorkGraphNodeResponse,
		WorkGraphResponse,
		WorkNodeDetailResponse
	} from '$lib/types/api';
	import { buildFlowGraph } from './layout';
	import RunGraphNode from './RunGraphNode.svelte';

	let {
		graph,
		inspectorSeedID,
		nodeDetail = null
	}: {
		graph: WorkGraphResponse;
		inspectorSeedID?: string;
		nodeDetail?: WorkNodeDetailResponse | null;
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

	function structuredTextSummary(
		value: WorkNodeDetailResponse['task'] | WorkNodeDetailResponse['output']
	): string {
		if (value.preview_text && value.preview_text.trim() !== '') {
			return value.preview_text;
		}
		if (value.plain_text && value.plain_text.trim() !== '') {
			return value.plain_text;
		}
		return 'No text recorded.';
	}
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

		{#if nodeDetail}
			<div class="gc-panel px-4 py-4">
				<div class="flex items-start justify-between gap-3">
					<div>
						<p class="gc-stamp">Focused node</p>
						<h3 class="gc-panel-title mt-3 text-[1rem]">{nodeDetail.agent_id}</h3>
					</div>
					<span class={`gc-badge ${nodeDetail.status_class}`}>{nodeDetail.status_label}</span>
				</div>

				<div class="mt-4 grid gap-3">
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">Task</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">
							{structuredTextSummary(nodeDetail.task)}
						</p>
					</div>
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">Output</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">
							{structuredTextSummary(nodeDetail.output)}
						</p>
					</div>
				</div>

				<div class="mt-4 grid gap-2 border-t-2 border-[var(--gc-border)] pt-4">
					<div class="flex items-center justify-between gap-3">
						<p class="gc-stamp">Model</p>
						<p class="gc-machine">{nodeDetail.model_display}</p>
					</div>
					<div class="flex items-center justify-between gap-3">
						<p class="gc-stamp">Tokens</p>
						<p class="gc-machine">{nodeDetail.token_summary}</p>
					</div>
					<div class="flex items-center justify-between gap-3">
						<p class="gc-stamp">Started</p>
						<p class="gc-machine">{nodeDetail.started_at_label}</p>
					</div>
					<div class="flex items-center justify-between gap-3">
						<p class="gc-stamp">Updated</p>
						<p class="gc-machine">{nodeDetail.last_activity_label}</p>
					</div>
					{#if nodeDetail.session_url && nodeDetail.session_short_id}
						<div class="flex items-center justify-between gap-3">
							<p class="gc-stamp">Session</p>
							<a href={resolve(nodeDetail.session_url)} class="gc-machine text-[var(--gc-cyan)]">
								{nodeDetail.session_short_id}
							</a>
						</div>
					{/if}
				</div>

				{#if nodeDetail.approval}
					<div class="mt-4 border-t-2 border-[var(--gc-border)] pt-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Approval</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">{nodeDetail.approval.tool_name}</p>
						{#if nodeDetail.approval.binding_summary}
							<p class="gc-machine mt-2">{nodeDetail.approval.binding_summary}</p>
						{/if}
						{#if nodeDetail.approval.reason}
							<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">{nodeDetail.approval.reason}</p>
						{/if}
					</div>
				{/if}

				{#if nodeDetail.logs && nodeDetail.logs.length > 0}
					<div class="mt-4 border-t-2 border-[var(--gc-border)] pt-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Logs</p>
						<div class="mt-3 grid gap-3">
							{#each nodeDetail.logs.slice(0, 3) as entry (entry.title + entry.created_at_label)}
								<div class="gc-panel-soft px-3 py-3">
									<p class="gc-stamp">{entry.title}</p>
									<p class="gc-machine mt-2 whitespace-pre-wrap">{entry.body}</p>
								</div>
							{/each}
						</div>
					</div>
				{/if}
			</div>
		{/if}

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
