<script lang="ts">
	import type { ChannelStatusItem } from '$lib/channels/status';

	let { connector }: { connector: ChannelStatusItem } = $props();

	const statusToneByClass: Record<string, string> = {
		'is-success': 'var(--gc-success)',
		'is-error': 'var(--gc-error)',
		'is-active': 'var(--gc-primary)',
		'is-muted': 'var(--gc-ink-3)'
	};

	const statusTone = $derived(statusToneByClass[connector.state_class] ?? 'var(--gc-ink-3)');
	const hasDeliveryPressure = $derived(
		connector.pending_count > 0 || connector.retrying_count > 0 || connector.terminal_count > 0
	);
</script>

<div class="gc-panel-soft px-5 py-4">
	<div class="flex items-start gap-4">
		<div class="min-w-0 flex-1">
			<div class="flex flex-wrap items-center gap-3">
				<span class="gc-stamp text-[var(--gc-ink-2)]">{connector.connector_id}</span>
				<span class="gc-badge" style={`border-color: ${statusTone}; color: ${statusTone};`}>
					{connector.state_label}
				</span>
				{#if connector.restart_suggested}
					<span class="gc-badge border-[var(--gc-warning)] text-[var(--gc-warning)]">RESTART</span>
				{/if}
			</div>
			<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">{connector.summary}</p>
		</div>
		{#if connector.checked_at_label}
			<time class="gc-machine shrink-0 text-[var(--gc-ink-3)]">{connector.checked_at_label}</time>
		{/if}
	</div>

	<div class="mt-4 grid gap-3 sm:grid-cols-3">
		<div>
			<p class="gc-stamp text-[var(--gc-ink-3)]">Pending deliveries</p>
			<p class="gc-copy mt-2 text-[var(--gc-ink)]">{connector.pending_count}</p>
		</div>
		<div>
			<p class="gc-stamp text-[var(--gc-ink-3)]">Retrying</p>
			<p class="gc-copy mt-2 text-[var(--gc-ink)]">{connector.retrying_count}</p>
		</div>
		<div>
			<p class="gc-stamp text-[var(--gc-ink-3)]">Terminal</p>
			<p class="gc-copy mt-2 text-[var(--gc-ink)]">{connector.terminal_count}</p>
		</div>
	</div>

	<p
		class={`gc-copy mt-4 ${hasDeliveryPressure ? 'text-[var(--gc-warning)]' : 'text-[var(--gc-ink-3)]'}`}
	>
		{#if hasDeliveryPressure}
			Delivery queue needs attention on this channel.
		{:else}
			Queue is clear for this channel.
		{/if}
	</p>
</div>
