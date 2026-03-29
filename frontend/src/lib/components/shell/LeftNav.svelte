<script lang="ts">
	import { IconChevronLeft, IconChevronRight, IconMoon, IconSun } from '@tabler/icons-svelte-runes';
	import type { BootstrapNavItem } from '$lib/types/api';
	import SurfaceIcon from '$lib/components/shell/SurfaceIcon.svelte';

	let {
		navigation,
		currentPath,
		expanded,
		onToggle,
		theme,
		onToggleTheme
	}: {
		navigation: BootstrapNavItem[];
		currentPath: string;
		expanded: boolean;
		onToggle: () => void;
		theme: 'dark' | 'light';
		onToggleTheme: () => void;
	} = $props();

	function isActive(href: string): boolean {
		return currentPath === href || currentPath.startsWith(`${href}/`);
	}

	const railWidth = $derived(expanded ? 'w-[200px]' : 'w-[56px]');
</script>

<aside
	class="{railWidth} sticky top-0 flex h-screen flex-col border-r border-r-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)] transition-[width] duration-180 ease-in-out"
	aria-label="Primary navigation"
>
	<!-- Logo mark -->
	<div
		class="flex h-[var(--gc-topbar-h)] shrink-0 items-center border-b border-b-[1.5px] border-[var(--gc-border)] px-3"
	>
		{#if expanded}
			<span class="gc-panel-title truncate text-[1rem] text-[var(--gc-ink)]">GistClaw</span>
		{:else}
			<div
				class="h-7 w-7 border border-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-canvas)]"
				aria-hidden="true"
			></div>
		{/if}
	</div>

	<!-- Nav items -->
	<nav
		class="flex flex-1 flex-col gap-0.5 overflow-x-hidden overflow-y-auto p-2"
		aria-label="Sections"
	>
		{#each navigation as item (item.href)}
			{@const active = isActive(item.href)}
			<a
				href={item.href}
				aria-current={active ? 'page' : undefined}
				title={expanded ? undefined : item.label}
				class="flex min-h-[40px] items-center gap-3 border border-[1.5px] px-2 transition-colors duration-120 {active
					? 'border-[var(--gc-primary)] bg-[var(--gc-surface-raised)] text-[var(--gc-ink)]'
					: 'border-transparent text-[var(--gc-ink-2)] hover:border-[var(--gc-border)] hover:text-[var(--gc-ink)]'}"
			>
				<span class="shrink-0">
					<SurfaceIcon surfaceID={item.id} />
				</span>
				{#if expanded}
					<span class="gc-stamp truncate">{item.label}</span>
				{/if}
			</a>
		{/each}
	</nav>

	<!-- Footer: theme toggle + rail toggle -->
	<div
		class="flex shrink-0 flex-col gap-0.5 border-t border-t-[1.5px] border-[var(--gc-border)] p-2"
	>
		<button
			onclick={onToggleTheme}
			aria-label={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
			class="flex min-h-[40px] w-full items-center gap-3 border border-[1.5px] border-transparent px-2 text-[var(--gc-ink-2)] transition-colors hover:border-[var(--gc-border)] hover:text-[var(--gc-ink)]"
		>
			<span class="inline-flex shrink-0 items-center justify-center">
				{#if theme === 'dark'}
					<IconSun aria-hidden="true" size={18} stroke={1.6} />
				{:else}
					<IconMoon aria-hidden="true" size={18} stroke={1.6} />
				{/if}
			</span>
			{#if expanded}
				<span class="gc-stamp">{theme === 'dark' ? 'Light' : 'Dark'}</span>
			{/if}
		</button>

		<button
			onclick={onToggle}
			aria-label={expanded ? 'Collapse navigation' : 'Expand navigation'}
			aria-expanded={expanded}
			aria-keyshortcuts="["
			class="flex min-h-[40px] w-full items-center gap-3 border border-[1.5px] border-transparent px-2 text-[var(--gc-ink-3)] transition-colors hover:border-[var(--gc-border)] hover:text-[var(--gc-ink-2)]"
		>
			<span class="inline-flex shrink-0 items-center justify-center">
				{#if expanded}
					<IconChevronLeft aria-hidden="true" size={16} stroke={1.8} />
				{:else}
					<IconChevronRight aria-hidden="true" size={16} stroke={1.8} />
				{/if}
			</span>
			{#if expanded}
				<span class="gc-stamp">Collapse</span>
			{/if}
		</button>
	</div>
</aside>
