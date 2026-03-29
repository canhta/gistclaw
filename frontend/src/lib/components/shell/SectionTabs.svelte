<script lang="ts">
	export interface Tab {
		id: string;
		label: string;
	}

	let {
		tabs,
		activeTab = $bindable(),
		onchange
	}: {
		tabs: Tab[];
		activeTab: string;
		onchange?: (id: string) => void;
	} = $props();

	function activate(id: string): void {
		activeTab = id;
		onchange?.(id);
	}

	function handleKeydown(e: KeyboardEvent, index: number): void {
		let nextIndex: number;
		if (e.key === 'ArrowRight') {
			nextIndex = (index + 1) % tabs.length;
		} else if (e.key === 'ArrowLeft') {
			nextIndex = (index - 1 + tabs.length) % tabs.length;
		} else if (e.key === 'Home') {
			nextIndex = 0;
		} else if (e.key === 'End') {
			nextIndex = tabs.length - 1;
		} else {
			return;
		}
		e.preventDefault();
		activate(tabs[nextIndex].id);
		// Move focus to newly active tab
		const tabEls = (e.currentTarget as HTMLElement)
			?.closest('[role="tablist"]')
			?.querySelectorAll<HTMLElement>('[role="tab"]');
		tabEls?.[nextIndex]?.focus();
	}
</script>

<div
	role="tablist"
	aria-label="Section tabs"
	class="flex border-b border-b-[1.5px] border-[var(--gc-border)] px-6"
>
	{#each tabs as tab, i (tab.id)}
		{@const isActive = activeTab === tab.id}
		<button
			role="tab"
			aria-selected={isActive}
			tabindex={isActive ? 0 : -1}
			onclick={() => activate(tab.id)}
			onkeydown={(e) => handleKeydown(e, i)}
			class="gc-stamp -mb-[1.5px] border-b-[1.5px] px-0 py-3 transition-colors {isActive
				? 'border-[var(--gc-primary)] text-[var(--gc-ink)]'
				: 'border-transparent text-[var(--gc-ink-3)] hover:text-[var(--gc-ink-2)]'} {i > 0
				? 'ml-5'
				: ''}"
		>
			{tab.label}
		</button>
	{/each}
</div>
