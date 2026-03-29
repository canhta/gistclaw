<script lang="ts">
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TabID = 'run-update' | 'restart-report';

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'run-update', label: 'Run Update' },
		{ id: 'restart-report', label: 'Restart Report' }
	];

	let activeTabOverride = $state<TabID | null>(null);

	function isTabID(value: string | null): value is TabID {
		return value === 'run-update' || value === 'restart-report';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'run-update';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const projectName = $derived(data.project?.active_name ?? 'No project');
	const projectPath = $derived(data.project?.active_path ?? 'No active project path');
</script>

<svelte:head>
	<title>Update | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Update</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-6">
		<div class="grid gap-4 xl:grid-cols-4">
			<SurfaceMetricCard
				label="Release Channel"
				value="Manual"
				detail="GitHub Releases and package docs remain the shipped update path."
				tone="warning"
			/>
			<SurfaceMetricCard
				label="Machine Restart"
				value="Deferred"
				detail="Restart capture and restart-reason reporting are not wired into the web UI yet."
			/>
			<SurfaceMetricCard
				label="Operator Flow"
				value="Review -> Apply"
				detail="Use release notes plus Config apply until the dedicated workflow exists."
			/>
			<SurfaceMetricCard label="Project" value={projectName} detail={projectPath} tone="accent" />
		</div>

		{#if activeTab === 'run-update'}
			<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.3fr)_minmax(18rem,0.8fr)]">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">RUN UPDATE</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Plan a controlled runtime update</h2>
					<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
						Update workflow is not connected to a backend yet. The shipped maintenance path is still
						release-driven: review the published artifact, apply config or installer changes, then
						bring the runtime back under observation.
					</p>

					<div class="mt-5 flex flex-wrap gap-3">
						<button type="button" disabled class="gc-action px-4 py-2 opacity-40">
							Check Release Notes
						</button>
						<button type="button" disabled class="gc-action gc-action-solid px-4 py-2 opacity-40">
							Run Update
						</button>
					</div>

					<div class="mt-5 grid gap-3">
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Current shipped path</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Use GitHub Releases, the Ubuntu installer docs, or the Homebrew path from the
								project README for controlled upgrades.
							</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">After update</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Return to Debug, Channels, and Chat to verify queue health, connector state, and
								live run behavior.
							</p>
						</div>
					</div>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">Maintenance note</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Keep restarts explicit</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						The roadmap calls for clearer maintenance workflows, but without bypassing the runtime
						boundary or inventing a fake updater.
					</p>

					<div class="mt-5 space-y-4">
						<div class="border-t border-[var(--gc-border)] pt-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Config</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Apply config changes in the Config section, then watch for the required restart
								path.
							</p>
						</div>
						<div class="border-t border-[var(--gc-border)] pt-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Docs</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Installer and packaging guidance remain the source of truth until the update surface
								is wired.
							</p>
						</div>
					</div>
				</section>
			</div>
		{:else}
			<div class="mt-6 grid gap-4 lg:grid-cols-2">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">RESTART REPORT</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Restart report</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink)]">No restart report captured yet.</p>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						When the maintenance workflow lands, this tab should show who triggered the restart,
						what version or config change caused it, and whether the runtime came back healthy.
					</p>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">CURRENT WORKAROUND</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Keep the evidence path simple</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						Apply with restart currently lives in Config. After a restart, confirm machine recovery
						in Debug and connector recovery in Channels until this report is backed by runtime
						events.
					</p>
				</section>
			</div>
		{/if}
	</div>
</div>
