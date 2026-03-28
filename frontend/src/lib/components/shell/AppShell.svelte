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
		title,
		description,
		inspectorTitle,
		inspectorItems = [],
		children
	}: {
		navigation: BootstrapNavItem[];
		project: BootstrapProjectResponse;
		currentPath: string;
		title: string;
		description: string;
		inspectorTitle: string;
		inspectorItems?: InspectorItem[];
		children?: Snippet;
	} = $props();

	function isActive(href: string): boolean {
		return currentPath === href || currentPath.startsWith(`${href}/`);
	}

	function inspectorToneClass(tone: InspectorItem['tone']): string {
		if (tone === 'accent') {
			return 'border-[var(--gc-cyan)]';
		}
		if (tone === 'warning') {
			return 'border-[var(--gc-orange)]';
		}
		return 'border-[var(--gc-border)]';
	}
</script>

<div class="min-h-screen bg-[var(--gc-canvas)] text-[var(--gc-ink)]">
	<div class="grid min-h-screen grid-cols-1 xl:grid-cols-[18rem_minmax(0,1fr)_22rem]">
		<aside
			class="border-b-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface)] xl:border-r-2 xl:border-b-0"
		>
			<div class="border-b-2 border-[var(--gc-border)] px-5 py-6">
				<div class="flex items-start gap-3">
					<img
						src={logo}
						alt="GistClaw logo"
						class="h-12 w-12 border-2 border-[var(--gc-border-strong)] bg-[var(--gc-canvas)] p-1"
					/>
					<div>
						<p class="gc-stamp">GistClaw</p>
						<p class="gc-machine mt-2">local-first control deck</p>
					</div>
				</div>
				<h1 class="gc-panel-title mt-3 text-[1.45rem]">{project.active_name}</h1>
				<p class="gc-machine mt-3 break-all">{project.active_path}</p>
			</div>

			<nav class="grid gap-2 px-3 py-4">
				{#each navigation as item (item.href)}
					<a
						href={item.href}
						aria-current={isActive(item.href) ? 'page' : undefined}
						class={`gc-panel-soft grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-4 px-4 py-3 transition-colors ${
							isActive(item.href)
								? 'border-[var(--gc-orange)] bg-[var(--gc-surface-raised)] text-[var(--gc-ink)]'
								: 'text-[var(--gc-text-secondary)] hover:border-[var(--gc-cyan)] hover:text-[var(--gc-ink)]'
						}`}
					>
						<SurfaceIcon surfaceID={item.id} />
						<span class="gc-stamp">{item.label}</span>
						<span class="gc-machine">{item.id}</span>
					</a>
				{/each}
			</nav>
		</aside>

		<main class="flex min-w-0 flex-col">
			<header class="border-b-2 border-[var(--gc-border-strong)] px-6 py-6 lg:px-8 lg:py-8">
				<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
					<p class="gc-stamp">Active surface</p>
					<h2 class="gc-page-title mt-3">{title}</h2>
					<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">{description}</p>
				</div>
			</header>

			<div class="flex-1 px-6 py-6 lg:px-8 lg:py-8">
				{#if children}
					{@render children()}
				{/if}
			</div>
		</main>

		<aside
			class="border-t-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface)] px-5 py-6 xl:border-t-0 xl:border-l-2"
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
