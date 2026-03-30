<script lang="ts">
	let { label, detail, command }: { label: string; detail: string; command: string } = $props();
</script>

<div class="border border-[var(--gc-border)] px-4 py-4">
	<p class="gc-stamp text-[var(--gc-ink-3)]">{label}</p>
	<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">{detail}</p>
	<pre
		class="gc-code mt-4 overflow-x-auto border border-[var(--gc-border)] bg-[var(--gc-canvas)] px-4 py-3">{command}</pre>
</div>
