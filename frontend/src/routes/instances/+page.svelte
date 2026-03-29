<script lang="ts">
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TabID = 'presence' | 'details';

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'presence', label: 'Presence' },
		{ id: 'details', label: 'Details' }
	];

	let activeTabOverride = $state<TabID | null>(null);

	function isTabID(value: string | null): value is TabID {
		return value === 'presence' || value === 'details';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'presence';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const projectName = $derived(data.project?.active_name ?? 'No project');
	const projectPath = $derived(data.project?.active_path ?? 'No active project path');
</script>

<svelte:head>
	<title>Instances | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Instances</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-6">
		<div class="grid gap-4 xl:grid-cols-4">
			<SurfaceMetricCard
				label="Presence Feed"
				value="Deferred"
				detail="GistClaw does not expose a dedicated instance presence endpoint yet."
				tone="warning"
			/>
			<SurfaceMetricCard
				label="Worker Sessions"
				value="Shipped"
				detail="Runtime-managed sessions already describe the active assistant and worker topology."
				tone="accent"
			/>
			<SurfaceMetricCard
				label="Connector Signal"
				value="Live"
				detail="Connector-managed presence already ships where the runtime can own its lifecycle."
			/>
			<SurfaceMetricCard label="Project" value={projectName} detail={projectPath} tone="accent" />
		</div>

		{#if activeTab === 'presence'}
			<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(18rem,0.75fr)]">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">PRESENCE</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
						Instance presence is waiting on a dedicated backend.
					</h2>
					<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
						The runtime already owns worker sessions and connector-driven status. It also ships
						runtime-managed typing and presence where that signal is trustworthy, but it does not
						yet project those facts into a single instance inventory surface.
					</p>

					<div class="mt-5 grid gap-3 md:grid-cols-2">
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Chat</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Use Chat to watch active work, live run state, and the front assistant path.
							</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Channels</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Use Channels to confirm connector health and live delivery pressure for external
								surfaces.
							</p>
						</div>
					</div>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">Operator note</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Keep presence runtime-owned</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						This page should eventually merge worker, connector, and machine presence without
						relying on frontend-only polling assumptions.
					</p>
				</section>
			</div>
		{:else}
			<div class="mt-6 grid gap-4 lg:grid-cols-2">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">DETAILS</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
						Instance detail remains derived from live surfaces.
					</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						The runtime already exposes session, queue, and health detail, but not a single
						per-instance record. This page should eventually consolidate those views without
						duplicating authority.
					</p>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">CURRENT SOURCES</p>
					<div class="mt-3 grid gap-3">
						<div class="border border-[var(--gc-border)] px-4 py-3">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Sessions</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-3">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Debug</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-3">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Recover</p>
						</div>
					</div>
				</section>
			</div>
		{/if}
	</div>
</div>
