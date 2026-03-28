<script lang="ts">
	let {
		label,
		value,
		detail,
		tone = 'default'
	}: {
		label: string;
		value: string;
		detail: string;
		tone?: 'default' | 'accent' | 'warning';
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

<article class={`gc-panel-soft px-4 py-4 ${borderClass}`}>
	<p class="gc-stamp">{label}</p>
	<p class="gc-value mt-3">{value}</p>
	<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">{detail}</p>
</article>
