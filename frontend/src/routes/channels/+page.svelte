<script lang="ts">
	import { resolve } from '$app/paths';
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
	const routes = $derived(data.channels?.routes);

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
			<div class="mx-auto w-full max-w-6xl">
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

				<section class="gc-panel-soft mt-6 px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">ROUTE DIRECTORY</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Route directory</h2>
					<p class="gc-copy mt-3 max-w-3xl text-[var(--gc-ink-2)]">
						Deep connector config still lives in Config, but the active and historical route
						bindings are a live channel concern. This board comes from the shipped `/api/routes`
						directory.
					</p>

					<form method="GET" class="mt-5 grid gap-4 xl:grid-cols-4">
						<input type="hidden" name="tab" value="settings" />

						<div class="flex flex-col gap-2">
							<label for="route-search" class="gc-stamp text-[var(--gc-ink-3)]">Search routes</label
							>
							<input
								id="route-search"
								name="route_q"
								value={routes?.filters.query ?? ''}
								placeholder="Search by session, agent, or external id"
								class="gc-control min-h-[2.75rem]"
							/>
						</div>

						<div class="flex flex-col gap-2">
							<label for="route-connector" class="gc-stamp text-[var(--gc-ink-3)]">Connector</label>
							<input
								id="route-connector"
								name="route_connector_id"
								value={routes?.filters.connector_id ?? ''}
								placeholder="telegram"
								class="gc-control min-h-[2.75rem]"
							/>
						</div>

						<div class="flex flex-col gap-2">
							<label for="route-status" class="gc-stamp text-[var(--gc-ink-3)]">Route status</label>
							<select id="route-status" name="route_status" class="gc-control min-h-[2.75rem]">
								<option value="" selected={routes?.filters.status === ''}>Active only</option>
								<option value="all" selected={routes?.filters.status === 'all'}>All routes</option>
								<option value="inactive" selected={routes?.filters.status === 'inactive'}>
									Inactive only
								</option>
							</select>
						</div>

						<div class="flex flex-col gap-2">
							<label for="route-limit" class="gc-stamp text-[var(--gc-ink-3)]">Route limit</label>
							<input
								id="route-limit"
								type="number"
								min="1"
								max="100"
								name="route_limit"
								value={String(routes?.filters.limit ?? 50)}
								class="gc-control min-h-[2.75rem]"
							/>
						</div>

						<div class="flex flex-wrap justify-end gap-3 xl:col-span-4">
							<a
								href={resolve('/channels?tab=settings')}
								class="gc-action gc-action-accent px-4 py-2"
							>
								Clear filters
							</a>
							<button type="submit" class="gc-action gc-action-solid px-4 py-2">
								Apply filters
							</button>
						</div>
					</form>

					<div class="mt-5 grid gap-3">
						{#if (routes?.items?.length ?? 0) > 0}
							{#each routes?.items ?? [] as route (route.id)}
								<article class="border border-[var(--gc-border)] px-4 py-4">
									<div class="flex flex-wrap items-start justify-between gap-3">
										<div>
											<p class="gc-panel-title text-[var(--gc-ink)]">{route.connector_id}</p>
											<p class="gc-copy mt-2 text-[var(--gc-ink)]">{route.external_id}</p>
											<p class="gc-copy mt-2 text-sm text-[var(--gc-ink-3)]">
												Session {route.session_id} • Agent {route.agent_id}
											</p>
										</div>
										<div class="flex flex-wrap gap-2 text-xs text-[var(--gc-ink-3)]">
											<span class="gc-chip">{route.status_label}</span>
											<span class="gc-chip">{route.role_label}</span>
											{#if route.thread_id}
												<span class="gc-chip">{route.thread_id}</span>
											{/if}
										</div>
									</div>

									<div class="mt-4 flex flex-wrap gap-x-5 gap-y-2">
										<p class="gc-copy text-sm text-[var(--gc-ink-3)]">
											Created {route.created_at_label}
										</p>
										{#if route.deactivated_at_label}
											<p class="gc-copy text-sm text-[var(--gc-ink-3)]">
												Deactivated {route.deactivated_at_label}
											</p>
										{/if}
										{#if route.deactivation_reason}
											<p class="gc-copy text-sm text-[var(--gc-ink-3)]">
												Reason {route.deactivation_reason}
											</p>
										{/if}
									</div>
								</article>
							{/each}
						{:else}
							<div class="border border-dashed border-[var(--gc-border)] px-4 py-5">
								<p class="gc-copy text-[var(--gc-ink)]">No routes matched the current filters.</p>
							</div>
						{/if}
					</div>

					<div class="mt-5 flex flex-wrap gap-3">
						{#if routes?.paging.prevHref}
							<form method="GET" action={resolve('/channels')}>
								<input type="hidden" name="tab" value="settings" />
								<input type="hidden" name="route_q" value={routes.filters.query} />
								<input
									type="hidden"
									name="route_connector_id"
									value={routes.filters.connector_id}
								/>
								<input type="hidden" name="route_status" value={routes.filters.status} />
								<input type="hidden" name="route_limit" value={String(routes.filters.limit)} />
								<input
									type="hidden"
									name="route_cursor"
									value={new URLSearchParams(routes.paging.prevHref.split('?')[1] ?? '').get(
										'route_cursor'
									) ?? ''}
								/>
								<input type="hidden" name="route_direction" value="prev" />
								<button type="submit" class="gc-action gc-action-accent px-4 py-2">
									Previous route page
								</button>
							</form>
						{/if}
						{#if routes?.paging.nextHref}
							<form method="GET" action={resolve('/channels')}>
								<input type="hidden" name="tab" value="settings" />
								<input type="hidden" name="route_q" value={routes.filters.query} />
								<input
									type="hidden"
									name="route_connector_id"
									value={routes.filters.connector_id}
								/>
								<input type="hidden" name="route_status" value={routes.filters.status} />
								<input type="hidden" name="route_limit" value={String(routes.filters.limit)} />
								<input
									type="hidden"
									name="route_cursor"
									value={new URLSearchParams(routes.paging.nextHref.split('?')[1] ?? '').get(
										'route_cursor'
									) ?? ''}
								/>
								<input type="hidden" name="route_direction" value="next" />
								<button type="submit" class="gc-action gc-action-solid px-4 py-2">
									Next route page
								</button>
							</form>
						{/if}
					</div>
				</section>
			</div>
		{/if}
	</div>
</div>
