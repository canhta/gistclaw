<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		title,
		subtitle,
		actions
	}: {
		title: string;
		subtitle?: string;
		actions?: Snippet;
	} = $props();
</script>

<header
	class="flex items-center justify-between border-b border-b-[1.5px] border-[var(--gc-border)] px-6 py-4"
>
	<div>
		<h1 class="gc-panel-title text-[var(--gc-ink)]">{title}</h1>
		{#if subtitle}
			<p class="gc-stamp mt-1 text-[var(--gc-ink-3)]">{subtitle}</p>
		{/if}
	</div>

	{#if actions}
		<div class="flex items-center gap-2">
			{@render actions()}
		</div>
	{/if}
</header>
