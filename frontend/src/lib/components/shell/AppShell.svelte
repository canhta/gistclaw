<!-- eslint-disable svelte/no-navigation-without-resolve -->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { BootstrapNavItem, BootstrapProjectResponse } from '$lib/types/api';
	import SurfaceIcon from '$lib/components/shell/SurfaceIcon.svelte';
	import logo from '$lib/assets/logo.svg';

	type InspectorItem = {
		label: string;
		value: string;
		tone?: 'default' | 'accent' | 'warning';
	};

	let {
		navigation,
		project,
		currentPath,
		inspectorTitle,
		inspectorItems = [],
		children
	}: {
		navigation: BootstrapNavItem[];
		project: BootstrapProjectResponse;
		currentPath: string;
		inspectorTitle: string;
		inspectorItems?: InspectorItem[];
		children?: Snippet;
	} = $props();

	let infoOpen = $state(false);

	function isActive(href: string): boolean {
		return currentPath === href || currentPath.startsWith(`${href}/`);
	}

	function inspectorToneClass(tone: InspectorItem['tone']): string {
		if (tone === 'accent') return 'border-[var(--gc-cyan)]';
		if (tone === 'warning') return 'border-[var(--gc-orange)]';
		return 'border-[var(--gc-border)]';
	}

	function desktopNavClass(href: string): string {
		return `gc-panel-soft grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-4 px-4 py-3 transition-colors ${
			isActive(href)
				? 'border-[var(--gc-orange)] bg-[var(--gc-surface-raised)] text-[var(--gc-ink)]'
				: 'text-[var(--gc-text-secondary)] hover:border-[var(--gc-cyan)] hover:text-[var(--gc-ink)]'
		}`;
	}

	function mobileNavClass(href: string): string {
		return `gc-panel-soft shrink-0 px-4 py-3 transition-colors ${
			isActive(href)
				? 'border-[var(--gc-orange)] bg-[var(--gc-surface-raised)] text-[var(--gc-ink)]'
				: 'text-[var(--gc-text-secondary)] hover:border-[var(--gc-cyan)] hover:text-[var(--gc-ink)]'
		}`;
	}
</script>

<div class="min-h-screen bg-[var(--gc-canvas)] text-[var(--gc-ink)]">
	<!-- Mobile header -->
	<div class="border-b-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface)] xl:hidden">
		<div class="flex items-center justify-between gap-4 px-4 py-3 sm:px-6">
			<div class="flex items-center gap-3">
				<img
					src={logo}
					alt="GistClaw logo"
					class="h-9 w-9 border-2 border-[var(--gc-border-strong)] bg-[var(--gc-canvas)] p-1"
				/>
				<p class="gc-panel-title text-[1rem]">{project.active_name}</p>
			</div>
			<button
				onclick={() => (infoOpen = !infoOpen)}
				aria-expanded={infoOpen}
				aria-label="System info"
				class="gc-action px-3 py-2 text-[var(--gc-text-secondary)]"
			>
				<span class="gc-stamp">{infoOpen ? 'Close' : 'Info'}</span>
			</button>
		</div>

		{#if infoOpen}
			<div class="border-t-2 border-[var(--gc-border)] px-4 py-4 sm:px-6">
				<div class="grid gap-3 sm:grid-cols-2">
					<div class="gc-panel-soft px-3 py-3">
						<p class="gc-stamp">Project path</p>
						<p class="gc-machine mt-2 break-all">{project.active_path}</p>
					</div>
					{#each inspectorItems as item (`mobile-info-${item.label}`)}
						<div class={`gc-panel-soft px-3 py-3 ${inspectorToneClass(item.tone)}`}>
							<p class="gc-stamp">{item.label}</p>
							<p class="gc-value mt-2 text-[1rem]">{item.value}</p>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<div class="border-t-2 border-[var(--gc-border)] px-4 py-3 sm:px-6">
			<div class="max-w-full overflow-x-auto overflow-y-hidden overscroll-x-contain">
				<nav aria-label="Primary navigation" class="flex min-w-max gap-3 pb-1">
					{#each navigation as item (item.href)}
						<a
							href={item.href}
							aria-current={isActive(item.href) ? 'page' : undefined}
							class={mobileNavClass(item.href)}
						>
							<div class="flex items-center gap-3">
								<SurfaceIcon surfaceID={item.id} />
								<span class="gc-stamp">{item.label}</span>
							</div>
						</a>
					{/each}
				</nav>
			</div>
		</div>
	</div>

	<!-- Desktop layout -->
	<div class="grid min-h-screen grid-cols-1 xl:grid-cols-[18rem_minmax(0,1fr)_22rem]">
		<!-- Left nav -->
		<aside
			class="hidden bg-[var(--gc-surface)] xl:flex xl:h-screen xl:flex-col xl:border-r-2 xl:border-[var(--gc-border-strong)]"
		>
			<div class="border-b-2 border-[var(--gc-border)] px-5 py-6">
				<div class="flex items-start gap-3">
					<img
						src={logo}
						alt="GistClaw logo"
						class="h-12 w-12 border-2 border-[var(--gc-border-strong)] bg-[var(--gc-canvas)] p-1"
					/>
					<div>
						<p class="gc-stamp">Control deck</p>
						<p class="gc-machine mt-2">Repo workbench</p>
					</div>
				</div>
				<h1 class="gc-panel-title mt-3 text-[1.45rem]">{project.active_name}</h1>
				<p class="gc-machine mt-3 break-all">{project.active_path}</p>
			</div>

			<nav
				aria-label="Primary navigation"
				class="grid flex-1 auto-rows-min gap-2 overflow-y-auto px-3 py-4"
			>
				{#each navigation as item (item.href)}
					<a
						href={item.href}
						aria-current={isActive(item.href) ? 'page' : undefined}
						class={desktopNavClass(item.href)}
					>
						<SurfaceIcon surfaceID={item.id} />
						<span class="gc-stamp">{item.label}</span>
					</a>
				{/each}
			</nav>
		</aside>

		<!-- Main content -->
		<main class="flex min-w-0 flex-col">
			<div class="flex-1 px-4 py-5 sm:px-6 sm:py-6 lg:px-8 lg:py-8">
				{#if children}
					{@render children()}
				{/if}
			</div>
		</main>

		<!-- Right inspector -->
		<aside
			class="hidden bg-[var(--gc-surface)] px-5 py-6 xl:block xl:h-screen xl:overflow-y-auto xl:border-l-2 xl:border-[var(--gc-border-strong)]"
		>
			<div class="gc-panel px-4 py-4">
				<p class="gc-stamp">{inspectorTitle}</p>
				<div class="mt-4 grid gap-3">
					{#each inspectorItems as item (`${item.label}-${item.value}`)}
						<div class={`gc-panel-soft px-3 py-3 ${inspectorToneClass(item.tone)}`}>
							<p class="gc-stamp">{item.label}</p>
							<p class="gc-value mt-2 text-[1.15rem]">{item.value}</p>
						</div>
					{/each}
				</div>
			</div>
		</aside>
	</div>
</div>
