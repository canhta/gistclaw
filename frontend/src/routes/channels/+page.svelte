<script lang="ts">
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import ConnectorRow from '$lib/components/channels/ConnectorRow.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TabID = 'status' | 'login' | 'settings';

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'status', label: 'Status' },
		{ id: 'login', label: 'Login' },
		{ id: 'settings', label: 'Settings' }
	];

	let activeTabOverride = $state<TabID | null>(null);

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'status';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const summary = $derived(data.channels?.summary);
	const connectors = $derived(data.channels?.items ?? []);

	function isTabID(value: string | null): value is TabID {
		return value === 'status' || value === 'login' || value === 'settings';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}
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
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-6">
		{#if activeTab === 'status'}
			<div class="grid gap-4 xl:grid-cols-4">
				<SurfaceMetricCard
					label="Live Channels"
					value={String(summary?.active_count ?? 0)}
					detail="Connectors currently reporting a healthy or active runtime state."
					tone="accent"
				/>
				<SurfaceMetricCard
					label="Pending Deliveries"
					value={String(summary?.pending_count ?? 0)}
					detail="Outbound messages waiting for their next connector attempt."
				/>
				<SurfaceMetricCard
					label="Retrying Deliveries"
					value={String(summary?.retrying_count ?? 0)}
					detail="Connector sends currently in retry backoff."
					tone="warning"
				/>
				<SurfaceMetricCard
					label="Terminal Deliveries"
					value={String(summary?.terminal_count ?? 0)}
					detail="Messages that exhausted delivery and need operator attention."
					tone="warning"
				/>
			</div>

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
				<div class="mt-6 grid gap-4">
					{#each connectors as connector (connector.connector_id)}
						<ConnectorRow {connector} />
					{/each}
				</div>
			{/if}
		{:else if activeTab === 'login'}
			<div class="mx-auto w-full max-w-5xl">
				<p class="gc-stamp text-[var(--gc-ink-3)]">LOGIN</p>
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Bring a channel online</h2>
				<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
					OpenClaw treats channel login as an operator task, not a hidden setup note. In GistClaw,
					credentials live in machine config and the live connection state returns to the Status
					tab.
				</p>

				<div class="mt-6 grid gap-4 lg:grid-cols-2">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Telegram bot</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink)]">
							Set the bot token in Config &gt; Channels, then restart the runtime so the gateway can
							reconnect and report health here.
						</p>
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">WhatsApp Web</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink)]">
							Use the host-side login flow for the WhatsApp connector, then return to Status to
							confirm the lane is running cleanly.
						</p>
					</section>
				</div>
			</div>
		{:else}
			<div class="mx-auto w-full max-w-4xl">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">SETTINGS</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Channel settings moved</h2>
					<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
						Deep connector configuration belongs in Config &gt; Channels. Use this page to supervise
						live channel state, delivery pressure, and restart flags without digging through config
						fields.
					</p>
					<p class="gc-copy mt-4 text-[var(--gc-ink)]">
						Restart suggestions currently raised: {summary?.restart_suggested_count ?? 0}
					</p>
				</section>
			</div>
		{/if}
	</div>
</div>
