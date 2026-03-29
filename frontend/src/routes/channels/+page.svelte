<script lang="ts">
	import { resolve } from '$app/paths';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
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
	const access = $derived(
		data.channels?.access ?? {
			notice: '',
			settings: null,
			surfaces: []
		}
	);
	const accessNotice = $derived(access.notice ?? '');
	const accessSettings = $derived(access.settings ?? null);
	const accessSurfaces = $derived(access.surfaces ?? []);
	const telegramSurface = $derived(
		accessSurfaces.find((surface) => surface.id === 'telegram') ?? null
	);
	const whatsappSurface = $derived(
		accessSurfaces.find((surface) => surface.id === 'whatsapp') ?? null
	);
	const telegramConnector = $derived(
		connectors.find((connector) => connector.connector_id === 'telegram') ?? null
	);
	const whatsappConnector = $derived(
		connectors.find((connector) => connector.connector_id === 'whatsapp') ?? null
	);
	const configuredChannelCount = $derived(
		accessSurfaces.filter((surface) => surface.configured).length
	);
	const readyChannelCount = $derived(
		accessSurfaces.filter((surface) => surface.credential_state === 'ready').length
	);
	const visibleRouteCount = $derived(routes?.items?.length ?? 0);
	const telegramToken = $derived(accessSettings?.machine?.telegram_token ?? '');
	const maskedTelegramToken = $derived(telegramToken.trim() !== '' ? telegramToken : 'Missing');
	const whatsAppWebhookPath = '/webhooks/whatsapp';

	const credentialToneByState: Record<string, string> = {
		ready: 'var(--gc-success)',
		missing: 'var(--gc-warning)',
		unused: 'var(--gc-ink-3)'
	};
	const runtimeToneByClass: Record<string, string> = {
		'is-success': 'var(--gc-success)',
		'is-error': 'var(--gc-error)',
		'is-active': 'var(--gc-primary)',
		'is-muted': 'var(--gc-ink-3)'
	};

	function isTabID(value: string | null): value is TabID {
		return value === 'status' || value === 'login' || value === 'settings';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	function credentialTone(value: string | undefined): string {
		return credentialToneByState[value ?? 'unused'] ?? 'var(--gc-ink-3)';
	}

	function runtimeTone(value: string | undefined): string {
		return runtimeToneByClass[value ?? 'is-muted'] ?? 'var(--gc-ink-3)';
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
			<div class="mx-auto w-full max-w-6xl">
				{#if accessNotice !== ''}
					<SurfaceMessage label="ACCESS" message={accessNotice} className="mb-4" />
				{/if}

				<p class="gc-stamp text-[var(--gc-ink-3)]">LOGIN</p>
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Channel access board</h2>
				<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
					This board reports the shipped Telegram and WhatsApp lanes using the daemon’s actual
					settings and runtime status. It shows whether each channel is configured,
					credential-ready, and actively reporting health right now.
				</p>

				<div class="mt-6 grid gap-4 xl:grid-cols-4">
					<SurfaceMetricCard
						label="Configured Channels"
						value={String(configuredChannelCount)}
						detail={`${readyChannelCount} credential-ready in this daemon.`}
						tone="accent"
					/>
					<SurfaceMetricCard
						label="Live Channels"
						value={String(summary?.active_count ?? 0)}
						detail="Connectors currently reporting an active runtime state."
					/>
					<SurfaceMetricCard
						label="Restart Flags"
						value={String(summary?.restart_suggested_count ?? 0)}
						detail="Channels asking for operator attention before they run cleanly again."
						tone="warning"
					/>
					<SurfaceMetricCard
						label="Pending Deliveries"
						value={String(summary?.pending_count ?? 0)}
						detail="Outbound messages waiting behind channel access or connector pressure."
					/>
				</div>

				<div class="mt-6 grid gap-4 lg:grid-cols-2">
					<section class="gc-panel-soft px-5 py-5">
						<div class="flex flex-wrap items-center gap-3">
							<p class="gc-panel-title text-[var(--gc-ink)]">Telegram</p>
							<span
								class="gc-badge"
								style={`border-color: ${credentialTone(telegramSurface?.credential_state)}; color: ${credentialTone(telegramSurface?.credential_state)};`}
							>
								{telegramSurface?.credential_state_label ?? 'unavailable'}
							</span>
							{#if telegramConnector}
								<span
									class="gc-badge"
									style={`border-color: ${runtimeTone(telegramConnector.state_class)}; color: ${runtimeTone(telegramConnector.state_class)};`}
								>
									{telegramConnector.state_label}
								</span>
							{/if}
						</div>
						<p class="gc-copy mt-3 text-[var(--gc-ink)]">
							{telegramSurface?.summary ?? 'Telegram access details are unavailable.'}
						</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
							{telegramSurface?.detail ??
								'Manage the bot token through the current machine settings flow.'}
						</p>

						<div class="mt-5 grid gap-4">
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Masked Telegram token</p>
								<p class="gc-copy mt-3 text-[var(--gc-ink)]">{maskedTelegramToken}</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Runtime lane</p>
								<p class="gc-copy mt-3 text-[var(--gc-ink)]">
									{telegramConnector?.summary ?? 'No runtime snapshot yet.'}
								</p>
								{#if telegramConnector?.checked_at_label}
									<p class="gc-machine mt-2">{telegramConnector.checked_at_label}</p>
								{/if}
							</div>
						</div>
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<div class="flex flex-wrap items-center gap-3">
							<p class="gc-panel-title text-[var(--gc-ink)]">WhatsApp</p>
							<span
								class="gc-badge"
								style={`border-color: ${credentialTone(whatsappSurface?.credential_state)}; color: ${credentialTone(whatsappSurface?.credential_state)};`}
							>
								{whatsappSurface?.credential_state_label ?? 'unavailable'}
							</span>
							{#if whatsappConnector}
								<span
									class="gc-badge"
									style={`border-color: ${runtimeTone(whatsappConnector.state_class)}; color: ${runtimeTone(whatsappConnector.state_class)};`}
								>
									{whatsappConnector.state_label}
								</span>
							{/if}
						</div>
						<p class="gc-copy mt-3 text-[var(--gc-ink)]">
							{whatsappSurface?.summary ?? 'WhatsApp access details are unavailable.'}
						</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
							{whatsappSurface?.detail ??
								'WhatsApp access stays operator-managed through runtime config.'}
						</p>

						<div class="mt-5 grid gap-4">
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Webhook surface</p>
								<p class="gc-machine mt-3 break-all">{whatsAppWebhookPath}</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Delivery queue</p>
								<p class="gc-copy mt-3 text-[var(--gc-ink)]">
									{whatsappConnector?.pending_count ?? 0} pending ·
									{` ${whatsappConnector?.retrying_count ?? 0}`} retrying ·
									{` ${whatsappConnector?.terminal_count ?? 0}`} terminal
								</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									{whatsappConnector?.summary ?? 'No runtime snapshot yet.'}
								</p>
							</div>
						</div>
					</section>
				</div>
			</div>
		{:else}
			<div class="mx-auto w-full max-w-6xl">
				{#if accessNotice !== ''}
					<SurfaceMessage label="SETTINGS" message={accessNotice} className="mb-4" />
				{/if}

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">SETTINGS</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Connector settings snapshot</h2>
					<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
						Read-only connector config posture for the current daemon. This keeps the masked
						Telegram token, WhatsApp webhook surface, and live restart pressure next to route
						inventory so the operator can verify channel wiring without guessing from prose.
					</p>

					<div class="mt-6 grid gap-4 xl:grid-cols-4">
						<SurfaceMetricCard
							label="Configured Surfaces"
							value={String(configuredChannelCount)}
							detail={`${readyChannelCount} credential-ready connectors.`}
							tone="accent"
						/>
						<SurfaceMetricCard
							label="Restart Flags"
							value={String(summary?.restart_suggested_count ?? 0)}
							detail="Connectors currently asking for a restart or credential repair."
							tone="warning"
						/>
						<SurfaceMetricCard
							label="Visible Routes"
							value={String(visibleRouteCount)}
							detail="Route directory rows visible under the current filters."
						/>
						<SurfaceMetricCard
							label="Pending Deliveries"
							value={String(summary?.pending_count ?? 0)}
							detail="Messages still waiting behind current connector pressure."
						/>
					</div>

					<div class="mt-6 grid gap-4 lg:grid-cols-2">
						<section class="border border-[var(--gc-border)] px-4 py-4">
							<div class="flex flex-wrap items-center gap-3">
								<p class="gc-panel-title text-[var(--gc-ink)]">Telegram config</p>
								<span
									class="gc-badge"
									style={`border-color: ${credentialTone(telegramSurface?.credential_state)}; color: ${credentialTone(telegramSurface?.credential_state)};`}
								>
									{telegramSurface?.credential_state_label ?? 'unavailable'}
								</span>
							</div>
							<p class="gc-stamp mt-3 text-[var(--gc-ink-3)]">Masked Telegram token</p>
							<p class="gc-copy mt-3 text-[var(--gc-ink)]">{maskedTelegramToken}</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
								{telegramConnector?.summary ??
									telegramSurface?.summary ??
									'No runtime snapshot yet.'}
							</p>
						</section>

						<section class="border border-[var(--gc-border)] px-4 py-4">
							<div class="flex flex-wrap items-center gap-3">
								<p class="gc-panel-title text-[var(--gc-ink)]">WhatsApp webhook</p>
								<span
									class="gc-badge"
									style={`border-color: ${credentialTone(whatsappSurface?.credential_state)}; color: ${credentialTone(whatsappSurface?.credential_state)};`}
								>
									{whatsappSurface?.credential_state_label ?? 'unavailable'}
								</span>
							</div>
							<p class="gc-machine mt-3 break-all">{whatsAppWebhookPath}</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
								{whatsappConnector?.summary ??
									whatsappSurface?.summary ??
									'No runtime snapshot yet.'}
							</p>
						</section>
					</div>
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
