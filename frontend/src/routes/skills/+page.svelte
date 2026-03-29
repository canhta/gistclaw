<script lang="ts">
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TabID = 'installed' | 'available' | 'credentials';

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'installed', label: 'Installed' },
		{ id: 'available', label: 'Available' },
		{ id: 'credentials', label: 'Credentials' }
	];

	let activeTabOverride = $state<TabID | null>(null);

	function isTabID(value: string | null): value is TabID {
		return value === 'installed' || value === 'available' || value === 'credentials';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'installed';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const projectName = $derived(data.project?.active_name ?? 'No project');
	const projectPath = $derived(data.project?.active_path ?? 'No active project path');
</script>

<svelte:head>
	<title>Skills | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Skills</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-6">
		<div class="grid gap-4 xl:grid-cols-4">
			<SurfaceMetricCard
				label="Extension Seams"
				value="3"
				detail="Tools, providers, and connectors are the shipped seams today."
				tone="accent"
			/>
			<SurfaceMetricCard
				label="Workflow"
				value="Repo-managed"
				detail="Skill control still lives in files and runtime seams, not in browser edits."
			/>
			<SurfaceMetricCard
				label="Credentials"
				value="Manual"
				detail="Secrets still move through config and deployment paths."
				tone="warning"
			/>
			<SurfaceMetricCard label="Project" value={projectName} detail={projectPath} tone="accent" />
		</div>

		{#if activeTab === 'installed'}
			<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.3fr)_minmax(18rem,0.8fr)]">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">INSTALLED</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
						Installed skills are managed in the repo today.
					</h2>
					<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
						GistClaw already ships real extension seams, but the browser surface does not own them
						yet. The current source of truth is still the checked-in runtime and team setup.
					</p>

					<div class="mt-5 grid gap-3 md:grid-cols-3">
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Tools</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Built-in capabilities, web fetch, Tavily, and MCP stdio plug into the tool seam.
							</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Providers</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Anthropic and OpenAI-compatible adapters already ship behind the provider seam.
							</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Connectors</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								Telegram, WhatsApp, and optional Zalo Personal stay runtime-owned rather than
								browser-managed.
							</p>
						</div>
					</div>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">Operator note</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Keep extension changes explicit</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						This page should eventually supervise extension state, but it should not bypass the
						runtime seams or invent a marketplace before those flows are real.
					</p>
				</section>
			</div>
		{:else if activeTab === 'available'}
			<div class="mt-6 grid gap-4 lg:grid-cols-2">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">AVAILABLE</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Available skills</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						No marketplace or install workflow is wired yet. Discovery remains a documentation and
						repo task until the session-first runtime is ready for higher-level extension workflows.
					</p>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">NEXT SHAPE</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Workspace-owned packs</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						When this surface grows up, it should describe what can be installed, what is already
						trusted in the current workspace, and what still needs operator approval.
					</p>
				</section>
			</div>
		{:else}
			<div class="mt-6 grid gap-4 lg:grid-cols-2">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">CREDENTIALS</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Keep secrets explicit</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						Provider keys and connector secrets still live in config. The browser should not pretend
						those credential flows are available until the runtime owns them cleanly.
					</p>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">STATUS</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Credentials UI is deferred</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						Use the existing machine config and deployment workflow for secrets rotation until this
						surface is backed by real runtime policy and storage rules.
					</p>
				</section>
			</div>
		{/if}
	</div>
</div>
