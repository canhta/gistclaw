<script lang="ts">
	import type { Snippet } from 'svelte';

	type Tone = 'accent' | 'warning' | 'solid';

	let {
		type = 'button',
		tone = 'accent',
		disabled = false,
		className = '',
		onclick,
		children
	}: {
		type?: 'button' | 'submit';
		tone?: Tone;
		disabled?: boolean;
		className?: string;
		onclick?: ((event: MouseEvent) => void) | undefined;
		children?: Snippet;
	} = $props();

	const toneClass = $derived.by(() => {
		switch (tone) {
			case 'warning':
				return 'border-[var(--gc-orange)] hover:bg-[rgba(255,105,34,0.12)]';
			case 'solid':
				return 'border-[var(--gc-orange)] bg-[var(--gc-orange)] text-[var(--gc-canvas)] hover:border-[var(--gc-orange-hover)] hover:bg-[var(--gc-orange-hover)]';
			default:
				return 'border-[var(--gc-cyan)] hover:bg-[rgba(83,199,240,0.1)]';
		}
	});
</script>

<button
	{type}
	{disabled}
	{onclick}
	class={`border-2 px-4 py-3 text-left text-sm font-[var(--gc-font-mono)] font-bold tracking-[0.18em] uppercase transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${toneClass} ${className}`}
>
	{@render children?.()}
</button>
