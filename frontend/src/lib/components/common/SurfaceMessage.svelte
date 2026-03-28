<script lang="ts">
	type Tone = 'accent' | 'error';

	let {
		label,
		message,
		tone = 'accent',
		className = ''
	}: {
		label: string;
		message: string;
		tone?: Tone;
		className?: string;
	} = $props();

	const borderClass = $derived(
		tone === 'error' ? 'border-[var(--gc-error)]' : 'border-[var(--gc-cyan)]'
	);
	const stampClass = $derived(
		tone === 'error' ? 'text-[var(--gc-error)]' : 'text-[var(--gc-cyan)]'
	);
</script>

<div class={`gc-panel-soft px-4 py-4 ${borderClass} ${className}`}>
	<p class={`gc-stamp ${stampClass}`}>{label}</p>
	<p class="gc-copy mt-3 text-[var(--gc-ink)]">{message}</p>
</div>
