<script lang="ts">
	let {
		label,
		title = '',
		message,
		actionHref = '',
		actionLabel = '',
		tone = 'default',
		className = ''
	}: {
		label: string;
		title?: string;
		message: string;
		actionHref?: string;
		actionLabel?: string;
		tone?: 'default' | 'accent' | 'warning';
		className?: string;
	} = $props();

	const borderClass = $derived.by(() => {
		switch (tone) {
			case 'accent':
				return 'border-[var(--gc-cyan)]';
			case 'warning':
				return 'border-[var(--gc-orange)]';
			default:
				return 'border-[var(--gc-border)]';
		}
	});
</script>

<div class={`gc-panel-soft gc-empty-state px-4 py-4 ${borderClass} ${className}`}>
	<p class="gc-stamp">{label}</p>
	{#if title}
		<h3 class="gc-panel-title mt-3 text-[1rem]">{title}</h3>
	{/if}
	<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">{message}</p>
	{#if actionHref !== '' && actionLabel !== ''}
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
		<a href={actionHref} class="gc-action gc-action-accent mt-5">
			{actionLabel}
		</a>
	{/if}
</div>
