<script lang="ts">
	import {
		IconLayoutSidebarRightCollapse,
		IconLayoutSidebarRightExpand
	} from '@tabler/icons-svelte-runes';
	import type { BootstrapProjectResponse } from '$lib/types/api';

	let {
		project,
		onToggleInspector,
		inspectorOpen
	}: {
		project: BootstrapProjectResponse;
		onToggleInspector: () => void;
		inspectorOpen: boolean;
	} = $props();
</script>

<header
	class="sticky top-0 z-10 flex h-[var(--gc-topbar-h)] shrink-0 items-center justify-between border-b border-b-[1.5px] border-[var(--gc-border)] bg-[var(--gc-surface)] px-4"
>
	<!-- Left: project name -->
	<div class="flex min-w-0 items-center gap-3">
		<span class="gc-machine truncate text-[var(--gc-ink-2)]">{project.active_name}</span>
	</div>

	<!-- Right: inspector toggle -->
	<div class="flex items-center gap-1">
		<button
			onclick={onToggleInspector}
			aria-label={inspectorOpen ? 'Collapse inspector' : 'Expand inspector'}
			class="flex h-8 w-8 items-center justify-center border border-[1.5px] border-transparent text-[var(--gc-ink-3)] transition-colors hover:border-[var(--gc-border)] hover:text-[var(--gc-ink-2)]"
		>
			{#if inspectorOpen}
				<IconLayoutSidebarRightCollapse aria-hidden="true" size={16} stroke={1.6} />
			{:else}
				<IconLayoutSidebarRightExpand aria-hidden="true" size={16} stroke={1.6} />
			{/if}
		</button>
	</div>
</header>
