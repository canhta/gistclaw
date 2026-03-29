<!-- eslint-disable svelte/no-navigation-without-resolve -->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { IconMenu2, IconMoon, IconSun, IconX } from '@tabler/icons-svelte-runes';
	import type { BootstrapNavItem, BootstrapProjectResponse } from '$lib/types/api';
	import SurfaceIcon from '$lib/components/shell/SurfaceIcon.svelte';

	let {
		navigation,
		project,
		currentPath,
		theme,
		onToggleTheme,
		children
	}: {
		navigation: BootstrapNavItem[];
		project: BootstrapProjectResponse;
		currentPath: string;
		theme: 'dark' | 'light';
		onToggleTheme: () => void;
		children?: Snippet;
	} = $props();

	let drawerOpen = $state(false);

	function isActive(href: string): boolean {
		return currentPath === href || currentPath.startsWith(`${href}/`);
	}

	function closeDrawer(): void {
		drawerOpen = false;
	}
</script>

<!-- Mobile top bar -->
<header
	class="sticky top-0 z-20 flex h-[var(--gc-topbar-h)] items-center justify-between border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)] px-4"
>
	<span class="gc-panel-title text-[1rem] text-[var(--gc-ink)]">{project.active_name}</span>
	<button
		onclick={() => (drawerOpen = !drawerOpen)}
		aria-label={drawerOpen ? 'Close navigation' : 'Open navigation'}
		aria-expanded={drawerOpen}
		class="flex h-8 w-8 items-center justify-center border border-[1.5px] border-transparent text-[var(--gc-ink-2)] transition-colors hover:border-[var(--gc-border)] hover:text-[var(--gc-ink)]"
	>
		{#if drawerOpen}
			<IconX aria-hidden="true" size={18} stroke={1.8} />
		{:else}
			<IconMenu2 aria-hidden="true" size={18} stroke={1.8} />
		{/if}
	</button>
</header>

<!-- Nav drawer -->
{#if drawerOpen}
	<!-- Backdrop -->
	<button
		onclick={closeDrawer}
		aria-label="Close navigation"
		class="fixed inset-0 z-30 bg-[var(--gc-canvas)] opacity-80"
	></button>

	<!-- Drawer panel -->
	<nav
		aria-label="Primary navigation"
		class="fixed top-0 left-0 z-40 flex h-full w-[240px] flex-col border-r border-r-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div
			class="flex h-[var(--gc-topbar-h)] items-center border-b border-b-[1.5px] border-[var(--gc-border)] px-4"
		>
			<span class="gc-panel-title text-[1rem] text-[var(--gc-ink)]">GistClaw</span>
		</div>

		<div class="flex flex-1 flex-col gap-0.5 overflow-y-auto p-2">
			{#each navigation as item (item.href)}
				{@const active = isActive(item.href)}
				<a
					href={item.href}
					onclick={closeDrawer}
					aria-current={active ? 'page' : undefined}
					class="flex min-h-[44px] items-center gap-3 border border-[1.5px] px-3 transition-colors {active
						? 'border-[var(--gc-primary)] bg-[var(--gc-surface-raised)] text-[var(--gc-ink)]'
						: 'border-transparent text-[var(--gc-ink-2)] hover:border-[var(--gc-border)] hover:text-[var(--gc-ink)]'}"
				>
					<SurfaceIcon surfaceID={item.id} />
					<span class="gc-stamp">{item.label}</span>
				</a>
			{/each}
		</div>

		<div class="shrink-0 border-t border-t-[1.5px] border-[var(--gc-border)] p-2">
			<button
				onclick={onToggleTheme}
				aria-label={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
				class="flex min-h-[44px] w-full items-center gap-3 border border-[1.5px] border-transparent px-3 text-[var(--gc-ink-2)] transition-colors hover:border-[var(--gc-border)] hover:text-[var(--gc-ink)]"
			>
				{#if theme === 'dark'}
					<IconSun aria-hidden="true" size={18} stroke={1.6} />
				{:else}
					<IconMoon aria-hidden="true" size={18} stroke={1.6} />
				{/if}
				<span class="gc-stamp">{theme === 'dark' ? 'Light mode' : 'Dark mode'}</span>
			</button>
		</div>
	</nav>
{/if}

<!-- Content -->
<main class="px-4 py-5">
	{#if children}
		{@render children()}
	{/if}
</main>
