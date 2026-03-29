<script lang="ts">
	import type { RecoverRuntimeHealthResponse } from '$lib/types/api';

	let { connector }: { connector: RecoverRuntimeHealthResponse } = $props();

	const statusToneByClass: Record<string, string> = {
		'is-success': 'var(--gc-success)',
		'is-error': 'var(--gc-error)',
		'is-active': 'var(--gc-primary)',
		'is-muted': 'var(--gc-ink-3)'
	};

	const statusTone = $derived(statusToneByClass[connector.state_class] ?? 'var(--gc-ink-3)');
</script>

<div class="flex items-center gap-4 border-b border-[var(--gc-border)] px-5 py-4">
	<div class="min-w-0 flex-1">
		<div class="flex items-center gap-3">
			<span class="gc-stamp text-[var(--gc-ink-2)]">{connector.connector_id}</span>
			<span class="gc-badge" style={`border-color: ${statusTone}; color: ${statusTone};`}>
				{connector.state_label}
			</span>
			{#if connector.restart_suggested}
				<span class="gc-badge border-[var(--gc-warning)] text-[var(--gc-warning)]">RESTART</span>
			{/if}
		</div>
		<p class="gc-copy mt-1 text-[var(--gc-ink-2)]">{connector.summary}</p>
	</div>
	{#if connector.checked_at_label}
		<time class="gc-machine shrink-0 text-[var(--gc-ink-3)]">{connector.checked_at_label}</time>
	{/if}
</div>
