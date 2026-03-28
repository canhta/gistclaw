<script lang="ts">
	import type { Snippet } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { surfaceForPath } from '$lib/config/surfaces';
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();
	const surface = $derived(surfaceForPath(data.currentPath));
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

{#if data.auth.authenticated && data.project}
	<AppShell
		navigation={data.navigation}
		project={data.project}
		currentPath={data.currentPath}
		title={surface.title}
		description={surface.description}
		inspectorTitle={surface.inspectorTitle}
		inspectorItems={surface.inspectorItems}
	>
		{@render children()}
	</AppShell>
{:else}
	<div class="min-h-screen bg-[var(--gc-canvas)] text-[var(--gc-ink)]">
		{@render children()}
	</div>
{/if}
