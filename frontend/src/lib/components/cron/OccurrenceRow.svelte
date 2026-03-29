<script lang="ts">
	import type { AutomateOccurrenceResponse } from '$lib/types/api';

	let { occurrence }: { occurrence: AutomateOccurrenceResponse } = $props();
</script>

<tr class="border-b border-[var(--gc-border)]">
	<td class="px-4 py-3 align-top">
		<span class="gc-stamp text-[var(--gc-ink-2)]">{occurrence.schedule_name}</span>
	</td>
	<td class="px-4 py-3 align-top">
		<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-3)]">
			{occurrence.status_label}
		</span>
	</td>
	<td class="px-4 py-3 align-top">
		<span class="gc-machine text-[var(--gc-ink-4)]">{occurrence.slot_at_label}</span>
	</td>
	<td class="px-4 py-3 align-top">
		<div class="flex flex-col gap-1">
			<span class="gc-machine text-[var(--gc-ink-4)]">{occurrence.updated_at_label}</span>
			{#if occurrence.error}
				<span class="gc-copy text-[var(--gc-error)]">{occurrence.error}</span>
			{/if}
		</div>
	</td>
</tr>
