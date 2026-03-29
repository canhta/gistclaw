<script lang="ts">
	import ConnectorRow from '$lib/components/channels/ConnectorRow.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = [
		{ id: 'status', label: 'Status' },
		{ id: 'login', label: 'Login' },
		{ id: 'settings', label: 'Settings' }
	];

	let activeTab = $state('status');

	const connectors = $derived(data.channels?.connectors ?? []);
</script>

<svelte:head>
	<title>Channels | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Channels</h1>
		</div>
		<SectionTabs {tabs} bind:activeTab />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
		{#if activeTab === 'status'}
			{#if connectors.length === 0}
				<div class="flex flex-1 items-center justify-center p-10">
					<div class="text-center">
						<p class="gc-stamp text-[var(--gc-ink-3)]">CHANNELS</p>
						<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No channels connected</p>
						<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
							Add a channel to receive messages.
						</p>
					</div>
				</div>
			{:else}
				<div>
					{#each connectors as connector (connector.connector_id)}
						<ConnectorRow {connector} />
					{/each}
				</div>
			{/if}
		{:else if activeTab === 'login'}
			<div class="px-6 py-6">
				<p class="gc-stamp text-[var(--gc-ink-3)]">LOGIN</p>
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Connect a channel</h2>
				<p class="gc-copy mt-3 max-w-lg text-[var(--gc-ink-2)]">
					Configure local connector credentials, then restart the runtime to reconnect the channel.
				</p>
			</div>
		{:else}
			<div class="flex flex-1 items-center justify-center p-10">
				<div class="text-center">
					<p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
					<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Channel settings</p>
					<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
						Per-channel configuration will be available here.
					</p>
				</div>
			</div>
		{/if}
	</div>
</div>
