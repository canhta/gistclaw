<script lang="ts">
	import type { Snippet } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import './layout.css';
	import logo from '$lib/assets/logo.svg';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	const showShell = $derived(
		data.auth.authenticated &&
			!!data.project &&
			!data.currentPath.startsWith('/onboarding') &&
			data.currentPath !== '/login'
	);
</script>

<svelte:head><link rel="icon" href={logo} /></svelte:head>

{#if showShell}
	<AppShell navigation={data.navigation} project={data.project!} currentPath={data.currentPath}>
		{@render children()}
	</AppShell>
{:else}
	<div class="min-h-screen bg-[var(--gc-canvas)] text-[var(--gc-ink)]">
		{@render children()}
	</div>
{/if}
