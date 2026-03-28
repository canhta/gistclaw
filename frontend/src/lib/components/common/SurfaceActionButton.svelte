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
				return 'gc-action-warning';
			case 'solid':
				return 'gc-action-solid';
			default:
				return 'gc-action-accent';
		}
	});
</script>

<button {type} {disabled} {onclick} class={`gc-action ${toneClass} ${className}`}>
	{@render children?.()}
</button>
