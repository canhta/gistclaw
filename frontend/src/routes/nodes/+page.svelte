<script lang="ts">
	import SurfaceLoadErrorPanel from '$lib/components/common/SurfaceLoadErrorPanel.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TabID = 'list' | 'capabilities';

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'list', label: 'List' },
		{ id: 'capabilities', label: 'Capabilities' }
	];

	let activeTabOverride = $state<TabID | null>(null);

	function isTabID(value: string | null): value is TabID {
		return value === 'list' || value === 'capabilities';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'list';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const projectName = $derived(data.project?.active_name ?? 'No project');
	const projectPath = $derived(data.project?.active_path ?? 'No active project path');
	const nodes = $derived(data.nodes);
	const nodesNotice = $derived(data.nodesLoadError || nodes?.notice || '');
</script>

<svelte:head>
	<title>Nodes | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Nodes</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-6">
		{#if nodes === null}
			{#if nodesNotice !== ''}
				<SurfaceMessage label="NODES" message={nodesNotice} className="mb-4" />
			{/if}
			<div class="mt-2">
				<SurfaceLoadErrorPanel
					label="NODES"
					title="Nodes board unavailable"
					detail="The browser could not load the node inventory feed from this daemon. Reload to retry."
				/>
			</div>
		{:else}
			{#if nodesNotice !== ''}
				<SurfaceMessage label="NODES" message={nodesNotice} className="mb-4" />
			{/if}

			<div class="grid gap-4 xl:grid-cols-4">
				<SurfaceMetricCard
					label="Configured Connectors"
					value={String(nodes.summary.connectors)}
					detail={`${nodes.summary.approval_nodes} approval-bound nodes visible`}
					tone="accent"
				/>
				<SurfaceMetricCard
					label="Healthy Connectors"
					value={String(nodes.summary.healthy_connectors)}
					detail={`${nodes.summary.connectors - nodes.summary.healthy_connectors} connectors need attention`}
				/>
				<SurfaceMetricCard
					label="Capability Tools"
					value={String(nodes.summary.capabilities)}
					detail={`${nodes.summary.run_nodes} recent run nodes loaded`}
				/>
				<SurfaceMetricCard label="Project" value={projectName} detail={projectPath} tone="accent" />
			</div>

			{#if activeTab === 'list'}
				<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">LIST</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Configured connector inventory</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							This inventory stays inside the shipped runtime boundary: configured connectors on one
							side, recent run nodes from the active project on the other.
						</p>

						<div class="mt-6 grid gap-4 lg:grid-cols-2">
							<section class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Connectors</p>
								{#if nodes.connectors.length === 0}
									<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">No configured connectors found.</p>
								{:else}
									<div class="mt-4 grid gap-4">
										{#each nodes.connectors as connector (connector.id)}
											<div
												class="border-t border-[var(--gc-border)] pt-4 first:border-t-0 first:pt-0"
											>
												<div class="flex items-center justify-between gap-3">
													<p class="gc-panel-title text-[var(--gc-ink)]">{connector.id}</p>
													<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
														{connector.state_label}
													</span>
												</div>
												<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
													{connector.exposure}{#if connector.aliases.length > 0}
														{` · aliases ${connector.aliases.join(', ')}`}
													{/if}
												</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink)]">{connector.summary}</p>
												{#if connector.checked_at_label !== ''}
													<p class="gc-machine mt-2">{connector.checked_at_label}</p>
												{/if}
												{#if connector.restart_suggested}
													<p class="gc-machine mt-2 text-[var(--gc-warning)]">restart suggested</p>
												{/if}
											</div>
										{/each}
									</div>
								{/if}
							</section>

							<section class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Recent run nodes</p>
								{#if nodes.runs.length === 0}
									<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">No recent run nodes found.</p>
								{:else}
									<div class="mt-4 grid gap-4">
										{#each nodes.runs as run (run.id)}
											<div
												class="border-t border-[var(--gc-border)] pt-4 first:border-t-0 first:pt-0"
											>
												<div class="flex items-center justify-between gap-3">
													<p class="gc-panel-title text-[var(--gc-ink)]">{run.short_id}</p>
													<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
														{run.status_label}
													</span>
												</div>
												<p class="gc-copy mt-2 text-[var(--gc-ink)]">{run.objective_preview}</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
													{run.kind} · {run.agent_id}
												</p>
												<p class="gc-machine mt-2">
													{run.started_at_label} · {run.updated_at_label}
												</p>
											</div>
										{/each}
									</div>
								{/if}
							</section>
						</div>
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Operator focus</p>
						<div class="mt-4 space-y-4">
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Connector posture</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Use this board to see which configured connectors are healthy, degraded, or asking
									for a restart.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Run-node pressure</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Recent run nodes give a quick inventory of root and worker activity without
									pretending there is a separate worker-control plane.
								</p>
							</div>
						</div>
					</section>
				</div>
			{:else}
				<div class="mt-6 grid gap-4 lg:grid-cols-[minmax(0,1.1fr)_minmax(18rem,0.9fr)]">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">CAPABILITIES</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
							Direct runtime capability tools
						</h2>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							These are the registered direct tools currently available through the shipped runtime
							seam. They describe what the runtime can ask connectors or the app boundary to do
							right now.
						</p>

						{#if nodes.capabilities.length === 0}
							<p class="gc-copy mt-4 text-[var(--gc-ink-2)]">
								No direct capability tools registered.
							</p>
						{:else}
							<div class="mt-5 grid gap-4">
								{#each nodes.capabilities as capability (capability.name)}
									<div class="border border-[var(--gc-border)] px-4 py-4">
										<div class="flex items-center justify-between gap-3">
											<p class="gc-machine text-[var(--gc-ink)]">{capability.name}</p>
											<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
												{capability.family}
											</span>
										</div>
										<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">{capability.description}</p>
									</div>
								{/each}
							</div>
						{/if}
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Capability posture</p>
						<div class="mt-4 space-y-4">
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Connector tools</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Connector tools cover inbox, directory, target resolution, send, and live status
									operations.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">App actions</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									App actions stay on the runtime boundary for operator-safe product actions.
								</p>
							</div>
						</div>
					</section>
				</div>
			{/if}
		{/if}
	</div>
</div>
