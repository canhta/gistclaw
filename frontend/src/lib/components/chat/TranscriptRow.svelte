<script lang="ts">
	import type { TranscriptRow } from '$lib/chat/types';
	import ToolCallCard from './ToolCallCard.svelte';

	let { row }: { row: TranscriptRow } = $props();
</script>

{#if row.role === 'user'}
	<div
		class="border-b border-b-[1px] border-[var(--gc-border)] bg-[var(--gc-surface-raised)] px-5 py-4"
	>
		<div class="mb-2 flex items-center justify-between gap-4">
			<span class="gc-stamp text-[var(--gc-ink-3)]">USER</span>
			<time class="gc-machine text-[var(--gc-ink-4)]" datetime={row.timestamp}
				>{new Date(row.timestamp).toLocaleTimeString([], {
					hour: '2-digit',
					minute: '2-digit'
				})}</time
			>
		</div>
		<p class="gc-copy text-[var(--gc-ink)]">{row.text}</p>
	</div>
{:else if row.role === 'agent'}
	<div class="border-b border-b-[1px] border-[var(--gc-border)] bg-[var(--gc-surface)] px-5 py-4">
		<div class="mb-2 flex items-center justify-between gap-4">
			<span class="gc-stamp text-[var(--gc-ink-2)]">AGENT</span>
			<time class="gc-machine text-[var(--gc-ink-4)]" datetime={row.timestamp}
				>{new Date(row.timestamp).toLocaleTimeString([], {
					hour: '2-digit',
					minute: '2-digit'
				})}</time
			>
		</div>
		{#if row.text}
			<p class="gc-copy whitespace-pre-wrap text-[var(--gc-ink)]">
				{row.text}{#if row.isStreaming}<span
						class="streaming inline-block h-[1em] w-[2px] translate-y-[2px] animate-pulse bg-[var(--gc-primary)]"
						aria-hidden="true"
					></span>{/if}
			</p>
		{:else if row.isStreaming}
			<span
				class="streaming inline-block h-[1em] w-[2px] translate-y-[2px] animate-pulse bg-[var(--gc-primary)]"
				aria-hidden="true"
			></span>
		{/if}
		{#each row.toolCalls as toolCall (toolCall.id)}
			<ToolCallCard {toolCall} />
		{/each}
	</div>
{:else}
	<div
		class="border-b border-b-[1px] border-[var(--gc-border)] bg-[var(--gc-signal-dim)] px-5 py-4"
	>
		<div class="mb-2 flex items-center justify-between gap-4">
			<span class="gc-stamp text-[var(--gc-signal)]">NOTE</span>
			<time class="gc-machine text-[var(--gc-ink-4)]" datetime={row.timestamp}
				>{new Date(row.timestamp).toLocaleTimeString([], {
					hour: '2-digit',
					minute: '2-digit'
				})}</time
			>
		</div>
		<p class="gc-copy text-[var(--gc-ink)]">{row.text}</p>
	</div>
{/if}
