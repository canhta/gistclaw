<script lang="ts">
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TabID = 'live-tail' | 'filters' | 'export';

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'live-tail', label: 'Live Tail' },
		{ id: 'filters', label: 'Filters' },
		{ id: 'export', label: 'Export' }
	];

	let activeTabOverride = $state<TabID | null>(null);

	function isTabID(value: string | null): value is TabID {
		return value === 'live-tail' || value === 'filters' || value === 'export';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'live-tail';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const projectName = $derived(data.project?.active_name ?? 'No project');
	const projectPath = $derived(data.project?.active_path ?? 'No active project path');
</script>

<svelte:head>
	<title>Logs | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Logs</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-6">
		<div class="grid gap-4 xl:grid-cols-4">
			<SurfaceMetricCard
				label="Tail Status"
				value="Offline"
				detail="The runtime log stream endpoint has not shipped yet."
				tone="warning"
			/>
			<SurfaceMetricCard
				label="Default Window"
				value="500 lines"
				detail="Prepared for the first live tail pass once the endpoint is available."
			/>
			<SurfaceMetricCard
				label="Auto-follow"
				value="Ready"
				detail="The operator flow is reserved for staying pinned to the newest log output."
			/>
			<SurfaceMetricCard label="Project" value={projectName} detail={projectPath} tone="accent" />
		</div>

		{#if activeTab === 'live-tail'}
			<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(18rem,0.8fr)]">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">LIVE TAIL</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
						Live tail is waiting on the runtime log stream.
					</h2>
					<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
						GistClaw does not expose a dedicated log stream endpoint yet. This surface still sets
						the operator contract now so filtering, auto-follow, and export can land without
						reshaping the page later.
					</p>

					<div class="mt-5 flex flex-wrap gap-3">
						<button type="button" disabled class="gc-action gc-action-solid px-4 py-2 opacity-40">
							Start Tail
						</button>
						<button type="button" disabled class="gc-action px-4 py-2 opacity-40">Pause</button>
						<button type="button" disabled class="gc-action px-4 py-2 opacity-40">
							Auto-follow
						</button>
					</div>

					<div class="mt-5 border border-[var(--gc-border)] bg-[var(--gc-canvas)] px-4 py-4">
						<div class="flex flex-wrap items-center justify-between gap-3">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Runtime output</p>
							<p class="gc-copy text-[var(--gc-ink-3)]">No source attached</p>
						</div>
						<p class="gc-copy mt-4 text-[var(--gc-ink)]">No log stream available yet.</p>
						<pre
							class="gc-code mt-4 overflow-x-auto text-[var(--gc-ink-3)]">Waiting for runtime log endpoint…</pre>
					</div>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">Operator note</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
						Use the live surfaces that exist today
					</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						Use Chat for active run events and Debug for connector and machine health until log
						streaming ships.
					</p>

					<div class="mt-5 space-y-4">
						<div class="border-t border-[var(--gc-border)] pt-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Chat</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Watch transcript events, tool output, and run headers while work is in flight.
							</p>
						</div>
						<div class="border-t border-[var(--gc-border)] pt-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Debug</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Check queue pressure, runtime health, and connector state without waiting for logs.
							</p>
						</div>
					</div>
				</section>
			</div>
		{:else if activeTab === 'filters'}
			<div class="mt-6 grid gap-4 lg:grid-cols-2">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">Filter toolbar</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Shape the next tail session</h2>
					<div class="mt-5 grid gap-4 md:grid-cols-2">
						<label class="block">
							<span class="gc-stamp text-[var(--gc-ink-3)]">Search logs</span>
							<input
								type="text"
								disabled
								placeholder="connector retry timeout"
								class="gc-control mt-2 w-full opacity-40"
							/>
						</label>
						<label class="block">
							<span class="gc-stamp text-[var(--gc-ink-3)]">Level</span>
							<select disabled class="gc-control mt-2 w-full opacity-40">
								<option>info, warn, error</option>
							</select>
						</label>
						<label class="block">
							<span class="gc-stamp text-[var(--gc-ink-3)]">Source</span>
							<select disabled class="gc-control mt-2 w-full opacity-40">
								<option>runtime, connectors, scheduler</option>
							</select>
						</label>
						<label class="block">
							<span class="gc-stamp text-[var(--gc-ink-3)]">Time Range</span>
							<select disabled class="gc-control mt-2 w-full opacity-40">
								<option>Last 15 minutes</option>
							</select>
						</label>
					</div>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">Default filter set</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Prepared for incident triage</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						The first wired version should open with a narrow, operator-safe filter set instead of a
						raw firehose.
					</p>

					<div class="mt-5 grid gap-3">
						<div class="border border-[var(--gc-border)] px-4 py-3">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Keep</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Error and warning levels, delivery retries, approval failures, scheduler misses.
							</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-3">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Hide by default</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Verbose provider traces, repeated heartbeats, and steady-state connector noise.
							</p>
						</div>
					</div>
				</section>
			</div>
		{:else}
			<div class="mt-6 grid gap-4 lg:grid-cols-2">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">Export buffer</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
						Capture the current investigation window
					</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						Export is designed around the same filtered tail session, so operators can hand off a
						precise slice of evidence instead of a whole machine log.
					</p>

					<div class="mt-5 flex flex-wrap gap-3">
						<button type="button" disabled class="gc-action gc-action-solid px-4 py-2 opacity-40">
							Download Current Buffer
						</button>
						<button type="button" disabled class="gc-action px-4 py-2 opacity-40">
							Copy JSONL
						</button>
					</div>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">Backend status</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
						Export depends on the same tail backend.
					</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						Once the log endpoint exists, export should preserve the active filters, the cursor
						window, and the source path that produced the evidence.
					</p>
				</section>
			</div>
		{/if}
	</div>
</div>
