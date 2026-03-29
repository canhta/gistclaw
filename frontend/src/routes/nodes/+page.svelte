<script lang="ts">
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
		<div class="grid gap-4 xl:grid-cols-4">
			<SurfaceMetricCard
				label="Inventory"
				value="Deferred"
				detail="GistClaw does not expose a dedicated node inventory endpoint yet."
				tone="warning"
			/>
			<SurfaceMetricCard
				label="Capability Registry"
				value="Shipped"
				detail="Direct runtime capabilities already exist under the generic seam."
				tone="accent"
			/>
			<SurfaceMetricCard
				label="Exec Surface"
				value="Run Graph"
				detail="Live node detail still comes from work graphs and recovery surfaces."
			/>
			<SurfaceMetricCard label="Project" value={projectName} detail={projectPath} tone="accent" />
		</div>

		{#if activeTab === 'list'}
			<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(18rem,0.75fr)]">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">LIST</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
						Node inventory is waiting on a dedicated backend.
					</h2>
					<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
						GistClaw already exposes live run nodes, but not a standalone machine inventory. This
						page should eventually unify worker availability, bound execution targets, and
						capability claims without inventing them ahead of the runtime.
					</p>

					<div class="mt-5 grid gap-3 md:grid-cols-3">
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Run Graph</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Use Chat to inspect active run nodes, path state, and node-level graph detail.
							</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Debug</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Use Debug for machine health, queue signal, and current runtime posture.
							</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Recovery</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Use Recover when node-linked work needs operator intervention or approval.
							</p>
						</div>
					</div>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">Operator note</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Keep node facts runtime-owned</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						This surface should eventually reflect durable runtime claims, not a frontend-only list
						of imagined workers or devices.
					</p>
				</section>
			</div>
		{:else}
			<div class="mt-6 grid gap-4 lg:grid-cols-2">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">CAPABILITIES</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
						Capability seams ship before node inventory.
					</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						The runtime already knows about transport and app capabilities. What is missing is the
						inventory and reporting layer that turns those claims into a node supervision surface.
					</p>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">SHIPPED EXAMPLES</p>
					<div class="mt-3 grid gap-3">
						<div class="border border-[var(--gc-border)] px-4 py-3">
							<p class="gc-machine text-[var(--gc-ink)]">connector_inbox_list</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-3">
							<p class="gc-machine text-[var(--gc-ink)]">connector_send</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-3">
							<p class="gc-machine text-[var(--gc-ink)]">system.run</p>
						</div>
					</div>
				</section>
			</div>
		{/if}
	</div>
</div>
