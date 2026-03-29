<script lang="ts">
	import { getContext } from 'svelte';
	import type { InspectorContent } from '$lib/shell/inspector.svelte';

	// Inspector content is injected by the active section page via context.
	// When nothing is set, it falls back to null (section shows its own summary
	// or the slot renders nothing).
	const content = getContext<() => InspectorContent | null>('gc:inspector');
	const inspectorContent = $derived(content ? content() : null);
</script>

<aside
	class="sticky top-0 h-screen w-[var(--gc-inspector-w)] shrink-0 overflow-y-auto border-l border-l-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	aria-label="Inspector"
	aria-live="polite"
>
	{#if inspectorContent}
		<!-- Object detail: set by the active page -->
		<div class="p-4">
			{#if inspectorContent.title}
				<p class="gc-eyebrow text-[var(--gc-ink-3)]">{inspectorContent.eyebrow ?? 'DETAIL'}</p>
				<h2 class="gc-panel-title mt-2 text-[var(--gc-ink)]">{inspectorContent.title}</h2>
			{/if}

			{#if inspectorContent.items && inspectorContent.items.length > 0}
				<div class="mt-4 flex flex-col gap-2">
					{#each inspectorContent.items as item (item.label)}
						<div
							class="gc-panel-soft px-3 py-3 {item.tone === 'primary'
								? 'border-[var(--gc-primary)]'
								: item.tone === 'signal'
									? 'border-[var(--gc-signal)]'
									: ''}"
						>
							<p class="gc-stamp">{item.label}</p>
							<p class="gc-machine mt-1 break-all">{item.value}</p>
						</div>
					{/each}
				</div>
			{/if}

			{#if inspectorContent.actions && inspectorContent.actions.length > 0}
				<div class="mt-4 flex flex-col gap-2">
					{#each inspectorContent.actions as action (action.label)}
						<button
							onclick={action.onclick}
							class="gc-action w-full justify-start {action.primary ? 'gc-action-warning' : ''}"
						>
							{action.label}
						</button>
					{/each}
				</div>
			{/if}
		</div>
	{:else}
		<!-- Section-level summary: no item selected -->
		<div class="flex h-full items-center justify-center p-6">
			<p class="gc-secondary text-center text-[var(--gc-ink-3)]">Select an item to inspect</p>
		</div>
	{/if}
</aside>
