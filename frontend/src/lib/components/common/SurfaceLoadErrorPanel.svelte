<script lang="ts">
	let {
		label = 'UNAVAILABLE',
		title,
		detail
	}: {
		label?: string;
		title: string;
		detail: string;
	} = $props();
</script>

<section class="gc-panel-soft px-5 py-5">
	<p class="gc-stamp text-[var(--gc-ink-3)]">{label}</p>
	<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">{title}</h2>
	<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">{detail}</p>
</section>
