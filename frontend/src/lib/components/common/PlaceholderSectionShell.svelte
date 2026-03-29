<script lang="ts">
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';

	export interface PlaceholderTab {
		id: string;
		label: string;
		title: string;
		description: string;
	}

	let {
		sectionTitle,
		tabs,
		currentPath
	}: {
		sectionTitle: string;
		tabs: PlaceholderTab[];
		currentPath?: string;
	} = $props();

	const tabOptions = $derived(tabs.map(({ id, label }) => ({ id, label })));
	const fallbackTabID = $derived(tabOptions[0]?.id ?? '');

	let activeTab = $state('');
	const activePanel = $derived(tabs.find((tab) => tab.id === activeTab) ?? tabs[0]);

	$effect(() => {
		if (!activeTab && fallbackTabID) {
			activeTab = fallbackTabID;
		}
	});
</script>

<div class="flex h-full flex-col overflow-hidden" data-current-path={currentPath}>
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">{sectionTitle}</h1>
		</div>
		<SectionTabs tabs={tabOptions} bind:activeTab />
	</div>

	<div class="flex min-h-0 flex-1 items-center justify-center p-10">
		<div class="text-center">
			<p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
			<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">{activePanel?.title}</p>
			<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">{activePanel?.description}</p>
		</div>
	</div>
</div>
