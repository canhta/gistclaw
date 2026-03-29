<script lang="ts">
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TabID = 'status' | 'health' | 'models' | 'events' | 'rpc';

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'status', label: 'Status' },
		{ id: 'health', label: 'Health' },
		{ id: 'models', label: 'Models' },
		{ id: 'events', label: 'Events' },
		{ id: 'rpc', label: 'RPC' }
	];

	const validTabIDs = new Set(tabs.map((tab) => tab.id));

	let activeTabOverride = $state<TabID | null>(null);
	let confirmRpcUnlock = $state(false);
	let rpcUnlocked = $state(false);

	function isTabID(value: string | null): value is TabID {
		return Boolean(value) && validTabIDs.has(value as TabID);
	}

	const requestedTab = $derived.by(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'status';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const settings = $derived(data.debug?.settings ?? null);
	const machine = $derived(settings?.machine ?? null);
	const work = $derived(data.debug?.work ?? null);
	const health = $derived(data.debug?.health ?? { connectors: [], runtime_connectors: [] });

	const statusToneByState: Record<string, string> = {
		healthy: 'var(--gc-success)',
		degraded: 'var(--gc-warning)',
		unknown: 'var(--gc-ink-3)'
	};

	function statusLabel(value: string): string {
		return value.replaceAll('_', ' ');
	}
</script>

<svelte:head>
	<title>Debug | GistClaw</title>
</svelte:head>

{#if confirmRpcUnlock}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-[color-mix(in_srgb,var(--gc-canvas)_84%,transparent)] px-4"
		role="dialog"
		aria-modal="true"
		aria-label="Confirm RPC access"
	>
		<div class="gc-panel max-w-md px-6 py-5">
			<p class="gc-stamp text-[var(--gc-error)]">HIGH RISK</p>
			<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Unlock RPC console?</h2>
			<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
				Manual RPC can mutate runtime state directly. GistClaw does not expose a safe RPC endpoint
				yet, so this only unlocks a placeholder panel.
			</p>
			<div class="mt-5 flex justify-end gap-3">
				<button
					type="button"
					onclick={() => (confirmRpcUnlock = false)}
					class="gc-action px-4 py-2 text-[var(--gc-ink-2)]"
				>
					Cancel
				</button>
				<button
					type="button"
					onclick={() => {
						confirmRpcUnlock = false;
						rpcUnlocked = true;
					}}
					class="gc-action gc-action-warning px-4 py-2"
				>
					Unlock
				</button>
			</div>
		</div>
	</div>
{/if}

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Debug</h1>
		</div>
		<SectionTabs
			{tabs}
			{activeTab}
			onchange={(id) => {
				if (isTabID(id)) {
					activeTabOverride = id;
				}
			}}
		/>
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
		{#if activeTab === 'status'}
			<div class="mx-auto w-full max-w-6xl px-6 py-6">
				<p class="gc-stamp text-[var(--gc-ink-3)]">STATUS</p>
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Gateway snapshot</h2>
				<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
					OpenClaw’s debug view starts with runtime snapshots. Here that means the current machine
					settings plus the work queue summary GistClaw already exposes.
				</p>

				<div class="mt-6 grid gap-4 lg:grid-cols-3">
					<section class="gc-panel-soft px-4 py-4 lg:col-span-2">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Queue</p>
						<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">
							{work?.queue_strip.headline ?? 'No work queue data'}
						</p>
						<div class="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
							<div>
								<p class="gc-stamp text-[var(--gc-ink-3)]">Root Runs</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">
									{work?.queue_strip.root_runs ?? 0}
								</p>
							</div>
							<div>
								<p class="gc-stamp text-[var(--gc-ink-3)]">Active</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">
									{work?.queue_strip.summary.active ?? 0}
								</p>
							</div>
							<div>
								<p class="gc-stamp text-[var(--gc-ink-3)]">Needs Approval</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">
									{work?.queue_strip.summary.needs_approval ?? 0}
								</p>
							</div>
							<div>
								<p class="gc-stamp text-[var(--gc-ink-3)]">Root Status</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">
									{work?.queue_strip.summary.root_status ?? 'unknown'}
								</p>
							</div>
						</div>
					</section>

					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Budget</p>
						<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">
							{machine?.rolling_cost_label ?? '—'}
						</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink-3)]">
							Token budget {machine?.per_run_token_budget ?? '—'} · daily cap
							{machine?.daily_cost_cap_usd ?? '—'} USD
						</p>
					</section>

					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Approval Mode</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink)]">{machine?.approval_mode_label ?? '—'}</p>
					</section>

					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Host Access</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink)]">
							{machine?.host_access_mode_label ?? '—'}
						</p>
					</section>

					<section class="gc-panel-soft px-4 py-4 lg:col-span-1">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Active Project</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink)]">{machine?.active_project_name ?? '—'}</p>
						<p class="gc-copy mt-2 font-mono text-sm text-[var(--gc-ink-3)]">
							{machine?.active_project_path ?? '—'}
						</p>
					</section>
				</div>
			</div>
		{:else if activeTab === 'health'}
			<div class="mx-auto w-full max-w-6xl px-6 py-6">
				<p class="gc-stamp text-[var(--gc-ink-3)]">HEALTH</p>
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Connector and delivery health</h2>
				<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
					Delivery queue counts come from `/api/deliveries/health`, paired with runtime connector
					snapshots when they are available.
				</p>

				<div class="mt-6 grid gap-4 lg:grid-cols-2">
					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Delivery Queue</p>
						{#if health.connectors.length === 0}
							<p class="gc-copy mt-3 text-[var(--gc-ink-3)]">No connector queue data.</p>
						{:else}
							<div class="mt-4 flex flex-col gap-3">
								{#each health.connectors as connector (connector.connector_id)}
									<div class="border-b border-[var(--gc-border)] pb-3 last:border-b-0 last:pb-0">
										<div class="flex items-center justify-between gap-4">
											<span class="gc-stamp text-[var(--gc-ink-2)]">{connector.connector_id}</span>
											<span class="gc-copy text-[var(--gc-ink-3)]">
												{connector.pending_count} pending
											</span>
										</div>
										<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
											{connector.retrying_count} retrying · {connector.terminal_count} terminal
										</p>
									</div>
								{/each}
							</div>
						{/if}
					</section>

					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Runtime Connectors</p>
						{#if health.runtime_connectors.length === 0}
							<p class="gc-copy mt-3 text-[var(--gc-ink-3)]">No runtime connector snapshots.</p>
						{:else}
							<div class="mt-4 flex flex-col gap-3">
								{#each health.runtime_connectors as connector (connector.connector_id)}
									<div class="border-b border-[var(--gc-border)] pb-3 last:border-b-0 last:pb-0">
										<div class="flex items-center gap-3">
											<span class="gc-stamp text-[var(--gc-ink-2)]">{connector.connector_id}</span>
											<span
												class="gc-badge"
												style={`border-color: ${statusToneByState[connector.state] ?? 'var(--gc-ink-3)'}; color: ${statusToneByState[connector.state] ?? 'var(--gc-ink-3)'};`}
											>
												{statusLabel(connector.state)}
											</span>
											{#if connector.restart_suggested}
												<span class="gc-badge border-[var(--gc-warning)] text-[var(--gc-warning)]"
													>RESTART</span
												>
											{/if}
										</div>
										<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">{connector.summary}</p>
									</div>
								{/each}
							</div>
						{/if}
					</section>
				</div>
			</div>
		{:else if activeTab === 'models'}
			<div class="flex flex-1 items-center justify-center p-10">
				<div class="text-center">
					<p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
					<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Model inventory</p>
					<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
						GistClaw does not expose a model catalog endpoint yet.
					</p>
				</div>
			</div>
		{:else if activeTab === 'events'}
			<div class="flex flex-1 items-center justify-center p-10">
				<div class="text-center">
					<p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
					<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Runtime events</p>
					<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
						Event inspection will land here once there is a section-level event feed.
					</p>
				</div>
			</div>
		{:else}
			<div class="mx-auto w-full max-w-4xl px-6 py-6">
				<p class="gc-stamp text-[var(--gc-ink-3)]">RPC</p>
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">RPC console</h2>
				<div class="gc-panel-soft mt-6 border-[var(--gc-error)] px-4 py-4">
					<p class="gc-stamp text-[var(--gc-error)]">High risk</p>
					<p class="gc-copy mt-3 text-[var(--gc-ink)]">
						Manual RPC can mutate runtime state directly. This section stays locked until you opt
						in.
					</p>
				</div>

				<div class="mt-5 flex items-center gap-3">
					<button
						type="button"
						onclick={() => (confirmRpcUnlock = true)}
						class="gc-action gc-action-warning px-4 py-2"
					>
						Unlock RPC Console
					</button>
					<p class="gc-copy text-[var(--gc-ink-3)]">No RPC endpoint is exposed yet.</p>
				</div>

				{#if rpcUnlocked}
					<div class="gc-panel-soft mt-6 px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">UNAVAILABLE</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							The console is armed, but GistClaw still has no safe manual RPC API to call from the
							web UI.
						</p>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</div>
