<!-- eslint-disable svelte/no-navigation-without-resolve -->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { browser } from '$app/environment';
	import type { BootstrapNavItem, BootstrapProjectResponse } from '$lib/types/api';
	import LeftNav from '$lib/components/shell/LeftNav.svelte';
	import MobileShell from '$lib/components/shell/MobileShell.svelte';
	import RightInspector from '$lib/components/shell/RightInspector.svelte';
	import TopBar from '$lib/components/shell/TopBar.svelte';

	type Theme = 'dark' | 'light';

	let {
		navigation,
		project,
		currentPath,
		children
	}: {
		navigation: BootstrapNavItem[];
		project: BootstrapProjectResponse;
		currentPath: string;
		children?: Snippet;
	} = $props();

	// ─── Theme ────────────────────────────────────────────────────────────────
	let theme = $state<Theme>(
		typeof document !== 'undefined'
			? ((document.documentElement.getAttribute('data-theme') as Theme | null) ?? 'dark')
			: 'dark'
	);

	$effect(() => {
		if (!browser) return;
		document.documentElement.setAttribute('data-theme', theme);
	});

	function toggleTheme(): void {
		theme = theme === 'dark' ? 'light' : 'dark';
		try {
			localStorage.setItem('gc-theme', theme);
		} catch {
			// localStorage unavailable (private browsing) — state not persisted
		}
	}

	// ─── Rail expand/collapse ─────────────────────────────────────────────────
	let railExpanded = $state(false);

	$effect(() => {
		if (!browser) return;
		try {
			const stored = localStorage.getItem('gc-rail-expanded');
			if (stored !== null) railExpanded = stored === 'true';
		} catch {
			// localStorage unavailable — use default collapsed state
		}
	});

	function toggleRail(): void {
		railExpanded = !railExpanded;
		try {
			localStorage.setItem('gc-rail-expanded', String(railExpanded));
		} catch {
			// localStorage unavailable — state not persisted
		}
	}

	// ─── Rail keyboard shortcut [ ─────────────────────────────────────────────
	function handleKeydown(e: KeyboardEvent): void {
		if (e.key === '[' && !e.ctrlKey && !e.metaKey && !e.altKey) {
			const tag = (e.target as HTMLElement)?.tagName?.toLowerCase();
			if (tag !== 'input' && tag !== 'textarea' && tag !== 'select') {
				toggleRail();
			}
		}
	}

	// ─── Inspector content ────────────────────────────────────────────────────
	let inspectorOpen = $state(true);

	function toggleInspector(): void {
		inspectorOpen = !inspectorOpen;
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="min-h-screen bg-[var(--gc-canvas)] text-[var(--gc-ink)]">
	<!-- Desktop layout -->
	<div class="hidden xl:flex xl:min-h-screen">
		<!-- Left nav rail -->
		<LeftNav
			{navigation}
			{currentPath}
			expanded={railExpanded}
			onToggle={toggleRail}
			{theme}
			onToggleTheme={toggleTheme}
		/>

		<!-- Workspace -->
		<div class="flex min-w-0 flex-1 flex-col">
			<TopBar {project} onToggleInspector={toggleInspector} {inspectorOpen} />
			<main class="flex-1 overflow-y-auto px-6 py-6">
				{#if children}
					{@render children()}
				{/if}
			</main>
		</div>

		<!-- Right inspector -->
		{#if inspectorOpen}
			<RightInspector />
		{/if}
	</div>

	<!-- Mobile layout -->
	<div class="xl:hidden">
		<MobileShell {navigation} {project} {currentPath} {theme} onToggleTheme={toggleTheme}>
			{#if children}
				{@render children()}
			{/if}
		</MobileShell>
	</div>
</div>
