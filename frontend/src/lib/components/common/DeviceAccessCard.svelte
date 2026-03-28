<script lang="ts">
	import SurfaceActionButton from '$lib/components/common/SurfaceActionButton.svelte';
	import type { SettingsDeviceResponse } from '$lib/types/api';

	let {
		label,
		device,
		busy = false,
		onrevoke,
		onblock,
		onunblock
	}: {
		label: string;
		device: SettingsDeviceResponse;
		busy?: boolean;
		onrevoke?: (() => void) | undefined;
		onblock?: (() => void) | undefined;
		onunblock?: (() => void) | undefined;
	} = $props();

	const statusLabel = $derived.by(() => {
		if (device.blocked) {
			return 'Blocked';
		}
		if (device.current) {
			return 'Current';
		}
		return 'Signed in';
	});
</script>

<article class="gc-panel-soft px-4 py-4">
	<div class="flex flex-wrap items-start justify-between gap-4">
		<div>
			<p class="gc-stamp">{label}</p>
			<h3 class="gc-panel-title mt-3 text-[1rem]">{device.primary_label}</h3>
			<p class="gc-machine mt-3">{device.secondary_line}</p>
		</div>
		<p
			class={`gc-chip ${device.blocked ? 'gc-chip-warning' : device.current ? 'gc-chip-accent' : ''}`}
		>
			{statusLabel}
		</p>
	</div>

	<div class="mt-5 grid gap-3 sm:grid-cols-2">
		<div>
			<p class="gc-stamp">Network</p>
			<p class="gc-machine mt-2 break-all">{device.details_ip || 'Unknown network'}</p>
		</div>
		<div>
			<p class="gc-stamp">Sessions</p>
			<p class="gc-value mt-2">{device.active_sessions}</p>
		</div>
	</div>

	<div class="mt-4 border-t-2 border-[var(--gc-border)] pt-4">
		<p class="gc-stamp">User agent</p>
		<p class="gc-machine mt-2 break-all">{device.details_user_agent || 'Unknown browser'}</p>
	</div>

	{#if onrevoke || onblock || onunblock}
		<div class="mt-5 flex flex-wrap gap-3">
			{#if onrevoke}
				<SurfaceActionButton
					tone="warning"
					disabled={busy}
					onclick={() => {
						onrevoke?.();
					}}
				>
					Revoke
				</SurfaceActionButton>
			{/if}

			{#if onblock}
				<SurfaceActionButton
					tone="warning"
					disabled={busy}
					onclick={() => {
						onblock?.();
					}}
				>
					Block
				</SurfaceActionButton>
			{/if}

			{#if onunblock}
				<SurfaceActionButton
					disabled={busy}
					onclick={() => {
						onunblock?.();
					}}
				>
					Unblock
				</SurfaceActionButton>
			{/if}
		</div>
	{/if}
</article>
