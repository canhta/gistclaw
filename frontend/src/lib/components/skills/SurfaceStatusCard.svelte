<script lang="ts">
	import type { ExtensionSurfaceResponse } from '$lib/types/api';

	let { surface }: { surface: ExtensionSurfaceResponse } = $props();

	const posture = $derived.by(() => {
		if (surface.active) return 'active';
		if (surface.configured) return 'configured';
		return 'available';
	});
</script>

<article class="border border-[var(--gc-border)] px-4 py-4">
	<div class="flex items-start justify-between gap-3">
		<div>
			<p class="gc-panel-title text-[var(--gc-ink)]">{surface.name}</p>
			<p class="gc-machine mt-2 text-[var(--gc-ink-3)]">{surface.kind} · {posture}</p>
		</div>
		<div class="flex flex-wrap justify-end gap-2">
			<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
				{surface.kind}
			</span>
			<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
				{surface.credential_state_label}
			</span>
		</div>
	</div>

	<p class="gc-copy mt-3 text-[var(--gc-ink)]">{surface.summary}</p>
	<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">{surface.detail}</p>
</article>
