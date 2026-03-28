<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	$effect(() => {
		void goto(resolve(data.auth.authenticated ? '/work' : '/login'), { replaceState: true });
	});
</script>

<svelte:head>
	<title>GistClaw</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center px-6">
	<div class="gc-panel max-w-xl px-6 py-5 text-center">
		<p class="gc-stamp">Routing</p>
		<h1 class="gc-panel-title mt-3">Opening the control deck</h1>
		<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
			GistClaw is loading the right surface for this browser session.
		</p>
	</div>
</div>
