<script lang="ts">
	import type { AutomateScheduleResponse } from '$lib/types/api';

	let {
		schedule,
		onEnable,
		onDisable,
		onRunNow
	}: {
		schedule: AutomateScheduleResponse;
		onEnable?: (id: string) => void;
		onDisable?: (id: string) => void;
		onRunNow?: (id: string) => void;
	} = $props();
</script>

<tr class="border-b border-[var(--gc-border)]">
	<td class="px-4 py-3 align-top">
		<div class="flex min-w-0 flex-col gap-1">
			<span class="gc-stamp text-[var(--gc-ink-2)]">{schedule.name}</span>
			{#if schedule.objective}
				<span class="gc-copy text-[var(--gc-ink-3)]">{schedule.objective}</span>
			{/if}
		</div>
	</td>
	<td class="px-4 py-3 align-top">
		<span class="gc-copy text-[var(--gc-ink-2)]">{schedule.cadence_label}</span>
	</td>
	<td class="px-4 py-3 align-top">
		<span
			class="gc-badge {schedule.enabled
				? 'border-[var(--gc-success)] text-[var(--gc-success)]'
				: 'border-[var(--gc-ink-3)] text-[var(--gc-ink-3)]'}"
		>
			{schedule.enabled_label}
		</span>
	</td>
	<td class="px-4 py-3 align-top">
		<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-3)]">
			{schedule.status_label}
		</span>
	</td>
	<td class="px-4 py-3 align-top">
		<span class="gc-machine text-[var(--gc-ink-4)]">{schedule.next_run_at_label}</span>
	</td>
	<td class="px-4 py-3 align-top">
		<div class="flex justify-end gap-2">
			{#if schedule.enabled}
				<button
					type="button"
					onclick={() => onDisable?.(schedule.id)}
					class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-3)]"
				>
					Disable
				</button>
			{:else}
				<button
					type="button"
					onclick={() => onEnable?.(schedule.id)}
					class="gc-badge border-[var(--gc-success)] text-[var(--gc-success)]"
				>
					Enable
				</button>
			{/if}
			<button
				type="button"
				onclick={() => onRunNow?.(schedule.id)}
				class="gc-badge border-[var(--gc-primary)] text-[var(--gc-primary)]"
			>
				Run Now
			</button>
		</div>
	</td>
</tr>
