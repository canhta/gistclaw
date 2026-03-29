<script lang="ts">
	import type { ToolCall } from '$lib/chat/types';

	let { toolCall }: { toolCall: ToolCall } = $props();

	let expanded = $state(false);

	function toggle(): void {
		expanded = !expanded;
	}
</script>

<div class="mt-2 border border-[1.5px] border-[var(--gc-border)] bg-[var(--gc-surface-elevated)]">
	<button
		onclick={toggle}
		aria-expanded={expanded}
		class="flex w-full items-center gap-3 px-3 py-2 text-left transition-colors hover:bg-[var(--gc-surface-raised)]"
	>
		<span class="gc-stamp text-[var(--gc-ink-3)]">TOOL</span>
		<span class="gc-stamp flex-1 truncate text-[var(--gc-ink-2)]">{toolCall.name}</span>
		{#if toolCall.status === 'completed'}
			<span class="gc-stamp text-[var(--gc-success)]">✓</span>
		{:else if toolCall.status === 'failed'}
			<span class="gc-stamp text-[var(--gc-error)]">✗</span>
		{:else}
			<span class="gc-stamp text-[var(--gc-ink-3)]">···</span>
		{/if}
	</button>

	{#if expanded}
		<div class="border-t border-t-[1.5px] border-[var(--gc-border)] px-3 py-2">
			{#if toolCall.inputJSON}
				<p class="gc-stamp mb-1 text-[var(--gc-ink-3)]">Input</p>
				<pre class="gc-code overflow-x-auto text-[var(--gc-ink-2)]">{toolCall.inputJSON}</pre>
			{/if}
			{#if toolCall.logs.length > 0}
				<p class="gc-stamp mt-3 mb-1 text-[var(--gc-ink-3)]">Logs</p>
				<pre class="gc-code overflow-x-auto text-[var(--gc-ink-2)]">{toolCall.logs.join('\n')}</pre>
			{/if}
			{#if toolCall.outputJSON}
				<p class="gc-stamp mt-3 mb-1 text-[var(--gc-ink-3)]">Output</p>
				<pre class="gc-code overflow-x-auto text-[var(--gc-ink-2)]">{toolCall.outputJSON}</pre>
			{/if}
		</div>
	{/if}
</div>
