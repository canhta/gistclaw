<script lang="ts">
	import { resolve } from '$app/paths';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import { requestJSON } from '$lib/http/client';
	import type { DebugRPCStatusResponse } from '$lib/types/api';
	import { summarizeModelUsage } from '$lib/work/models';
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
	let rpcState = $state<DebugRPCStatusResponse | null>(null);
	let rpcMessage = $state('');
	let rpcError = $state('');
	let rpcBusy = $state(false);

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
	const runs = $derived.by(() => {
		const clusters = work?.clusters ?? [];
		return clusters.flatMap((cluster) => [cluster.root, ...(cluster.children ?? [])]);
	});
	const modelUsage = $derived(summarizeModelUsage(work?.clusters));
	const eventBoard = $derived(data.debug?.events ?? null);
	const eventSources = $derived(eventBoard?.sources ?? []);
	const eventEntries = $derived(eventBoard?.events ?? []);
	const selectedEventSource = $derived(
		eventSources.find((source) => source.run_id === eventBoard?.summary.selected_run_id) ?? null
	);
	const rpc = $derived(rpcState ?? data.debug?.rpc ?? null);
	const rpcProbes = $derived(rpc?.probes ?? []);
	const selectedRpcProbeName = $derived(
		rpc?.result.probe ?? rpc?.summary.selected_probe ?? rpc?.summary.default_probe ?? 'status'
	);
	const selectedRpcProbe = $derived(
		rpcProbes.find((probe) => probe.name === selectedRpcProbeName) ?? null
	);
	const defaultRpcProbe = $derived(
		rpcProbes.find((probe) => probe.name === rpc?.summary.default_probe) ?? null
	);
	const rpcPayload = $derived.by(() => JSON.stringify(rpc?.result.data ?? {}, null, 2));

	const statusToneByState: Record<string, string> = {
		healthy: 'var(--gc-success)',
		degraded: 'var(--gc-warning)',
		unknown: 'var(--gc-ink-3)'
	};

	function statusLabel(value: string): string {
		return value.replaceAll('_', ' ');
	}

	function probeLabel(name: string | undefined): string {
		if (!name) {
			return '—';
		}
		return rpcProbes.find((probe) => probe.name === name)?.label ?? name.replaceAll('_', ' ');
	}

	async function refreshRPCProbe(name: string): Promise<void> {
		rpcBusy = true;
		rpcMessage = '';
		rpcError = '';

		try {
			const query = new URLSearchParams({ probe: name });
			rpcState = await requestJSON<DebugRPCStatusResponse>(
				globalThis.fetch.bind(globalThis),
				`/api/debug/rpc?${query.toString()}`
			);
			rpcMessage = `${rpcState.result.label} probe refreshed.`;
		} catch (err) {
			rpcError = err instanceof Error ? err.message : 'Failed to refresh the selected probe.';
		} finally {
			rpcBusy = false;
		}
	}
</script>

<svelte:head>
	<title>Debug | GistClaw</title>
</svelte:head>

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
			<div class="mx-auto w-full max-w-6xl px-6 py-6">
				<p class="gc-stamp text-[var(--gc-ink-3)]">MODELS</p>
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Model usage</h2>
				<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
					GistClaw does not expose a full model catalog yet, but the current work queue already
					tells you which models are active. Use this tab to spot the models currently attached to
					live or recent runs before you change config defaults elsewhere.
				</p>

				<div class="mt-6 grid gap-4 xl:grid-cols-4">
					<SurfaceMetricCard
						label="Observed Runs"
						value={String(runs.length)}
						detail={`${runs.length} runs sampled from the current queue.`}
					/>
					<SurfaceMetricCard
						label="Distinct Models"
						value={String(modelUsage.length)}
						detail="Unique model displays attached to the visible queue."
						tone="accent"
					/>
					<SurfaceMetricCard
						label="Token Budget"
						value={machine?.per_run_token_budget ?? '—'}
						detail="Current per-run budget from runtime settings."
					/>
					<SurfaceMetricCard
						label="Daily Cap"
						value={machine?.daily_cost_cap_usd ?? '—'}
						detail="Daily spend cap from runtime settings."
						tone="warning"
					/>
				</div>

				<section class="gc-panel-soft mt-6 px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">Active model displays</p>
					{#if modelUsage.length === 0}
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">No run models sampled yet.</p>
					{:else}
						<div class="mt-4 flex flex-col gap-4">
							{#each modelUsage as item (item.model)}
								<div class="border-b border-[var(--gc-border)] pb-4 last:border-b-0 last:pb-0">
									<div class="flex items-center justify-between gap-4">
										<p class="gc-panel-title text-[var(--gc-ink)]">{item.model}</p>
										<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
											{item.count}
											{item.count === 1 ? 'run' : 'runs'}
										</span>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</section>
			</div>
		{:else if activeTab === 'events'}
			<div class="mx-auto w-full max-w-6xl px-6 py-6">
				<p class="gc-stamp text-[var(--gc-ink-3)]">EVENTS</p>
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Recent event log</h2>
				<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
					This board pulls recent replay events for the active project and keeps the live SSE source
					for the selected run visible. Use it to inspect the latest run activity without leaving
					Debug.
				</p>

				<div class="mt-6 grid gap-4 xl:grid-cols-3">
					<SurfaceMetricCard
						label="Live Sources"
						value={String(eventBoard?.summary.source_count ?? eventSources.length)}
						detail="Recent runs in the active project that already have replay events."
					/>
					<SurfaceMetricCard
						label="Loaded Events"
						value={String(eventBoard?.summary.event_count ?? 0)}
						detail="Recent events loaded for the selected run."
						tone="warning"
					/>
					<SurfaceMetricCard
						label="Latest Event"
						value={eventBoard?.summary.latest_event_label ?? 'No events yet'}
						detail={eventBoard?.summary.latest_event_at_label ?? 'No replay activity sampled yet.'}
						tone="accent"
					/>
				</div>

				<div class="mt-6 grid gap-5 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,0.9fr)]">
					<section class="gc-panel px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">SSE sources</p>
						{#if eventSources.length === 0}
							<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
								No replay-backed runs are visible right now.
							</p>
						{:else}
							<div class="mt-4 flex flex-col gap-4">
								{#each eventSources as source (source.run_id)}
									<div class="border-b border-[var(--gc-border)] pb-4 last:border-b-0 last:pb-0">
										<div class="flex items-start justify-between gap-4">
											<div>
												<div class="flex flex-wrap items-center gap-2">
													<p class="gc-panel-title text-[var(--gc-ink)]">{source.objective}</p>
													{#if source.run_id === eventBoard?.summary.selected_run_id}
														<span
															class="gc-badge border-[var(--gc-primary)] text-[var(--gc-primary)]"
														>
															SELECTED
														</span>
													{/if}
												</div>
												<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
													{source.run_id} · {source.agent_id}
												</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
													{source.event_count} events · {source.latest_event_at_label}
												</p>
											</div>
											<a
												href={resolve(
													`/debug?tab=events&run_id=${encodeURIComponent(source.run_id)}`
												)}
												class="gc-action px-4 py-2"
											>
												View
											</a>
										</div>
										<p class="gc-copy mt-3 font-mono text-sm break-all text-[var(--gc-signal)]">
											{source.stream_url}
										</p>
									</div>
								{/each}
							</div>
						{/if}
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Recent event log</p>
						{#if selectedEventSource}
							<div class="mt-4 border-b border-[var(--gc-border)] pb-4">
								<p class="gc-panel-title text-[var(--gc-ink)]">{selectedEventSource.objective}</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									{selectedEventSource.run_id} · {selectedEventSource.agent_id}
								</p>
								<p class="gc-copy mt-2 font-mono text-sm break-all text-[var(--gc-signal)]">
									{selectedEventSource.stream_url}
								</p>
							</div>
						{/if}

						{#if eventEntries.length === 0}
							<p class="gc-copy mt-4 text-[var(--gc-ink-2)]">No events loaded for this run yet.</p>
						{:else}
							<div class="mt-4 flex flex-col gap-4">
								{#each eventEntries as event (event.id)}
									<div class="border-b border-[var(--gc-border)] pb-4 last:border-b-0 last:pb-0">
										<div class="flex flex-wrap items-center gap-3">
											<p class="gc-panel-title text-[var(--gc-ink)]">{event.kind_label}</p>
											<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
												{event.run_short_id}
											</span>
										</div>
										<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
											{event.objective} · {event.agent_id}
										</p>
										<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">{event.occurred_at_label}</p>
										<p class="gc-copy mt-3 font-mono text-sm break-all text-[var(--gc-signal)]">
											{event.payload_preview}
										</p>
									</div>
								{/each}
							</div>
						{/if}
					</section>
				</div>
			</div>
		{:else}
			<div class="mx-auto w-full max-w-6xl px-6 py-6">
				<p class="gc-stamp text-[var(--gc-ink-3)]">RPC</p>
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">RPC probes</h2>
				<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
					Read-only app probes run through GistClaw&apos;s existing capability seam. Each probe is
					whitelisted, returns structured JSON, and never mutates runtime state.
				</p>

				<div class="mt-6 grid gap-4 xl:grid-cols-4">
					<SurfaceMetricCard
						label="Safe Probes"
						value={String(rpc?.summary.probe_count ?? 0)}
						detail="Whitelisted probes available from the current daemon."
					/>
					<SurfaceMetricCard
						label="Mode"
						value={rpc?.summary.read_only ? 'Read-only' : 'Unavailable'}
						detail="This surface does not expose raw mutation RPC."
						tone="accent"
					/>
					<SurfaceMetricCard
						label="Selected"
						value={selectedRpcProbe?.label ?? probeLabel(selectedRpcProbeName)}
						detail={rpc?.result.summary ?? 'No probe result loaded yet.'}
					/>
					<SurfaceMetricCard
						label="Default"
						value={defaultRpcProbe?.label ?? probeLabel(rpc?.summary.default_probe)}
						detail="Default probe loaded when no specific probe is requested."
					/>
				</div>

				{#if rpcMessage}
					<div class="mt-5">
						<SurfaceMessage label="REFRESHED" message={rpcMessage} />
					</div>
				{/if}

				{#if rpcError}
					<div class="mt-5">
						<SurfaceMessage label="PROBE ERROR" message={rpcError} tone="error" />
					</div>
				{/if}

				{#if rpc?.notice}
					<div class="mt-5">
						<SurfaceMessage label="RPC" message={rpc.notice} />
					</div>
				{/if}

				<div class="mt-6 grid gap-5 xl:grid-cols-[minmax(0,0.95fr)_minmax(0,1.15fr)]">
					<section class="gc-panel px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Probe catalog</p>
						<div class="mt-4 flex flex-col gap-4">
							{#each rpcProbes as probe (probe.name)}
								<div class="border-b border-[var(--gc-border)] pb-4 last:border-b-0 last:pb-0">
									<div class="flex items-start justify-between gap-4">
										<div>
											<div class="flex flex-wrap items-center gap-2">
												<p class="gc-panel-title text-[var(--gc-ink)]">{probe.label}</p>
												{#if selectedRpcProbeName === probe.name}
													<span
														class="gc-badge border-[var(--gc-primary)] text-[var(--gc-primary)]"
													>
														SELECTED
													</span>
												{/if}
											</div>
											<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">{probe.description}</p>
											<p class="gc-machine mt-3 text-[var(--gc-ink-4)]">{probe.name}</p>
										</div>
										<button
											type="button"
											onclick={() => void refreshRPCProbe(probe.name)}
											class="gc-action px-4 py-2"
											disabled={rpcBusy}
										>
											{rpcBusy && selectedRpcProbeName === probe.name ? 'Refreshing…' : 'Run probe'}
										</button>
									</div>
								</div>
							{/each}
						</div>
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Result</p>
						<div class="mt-4 flex flex-wrap items-center gap-3">
							<p class="gc-panel-title text-[var(--gc-ink)]">
								{selectedRpcProbe?.label ?? rpc.result.label}
							</p>
							<span class="gc-badge border-[var(--gc-cyan)] text-[var(--gc-cyan)]">READ-ONLY</span>
						</div>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">{rpc.result.summary}</p>
						<div class="mt-4 grid gap-3 sm:grid-cols-2">
							<div>
								<p class="gc-stamp text-[var(--gc-ink-3)]">Last run</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">{rpc.result.executed_at_label}</p>
							</div>
							<div>
								<p class="gc-stamp text-[var(--gc-ink-3)]">Probe</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">{rpc.result.probe}</p>
							</div>
						</div>

						<div class="mt-5">
							<p class="gc-stamp text-[var(--gc-ink-3)]">JSON</p>
							<pre
								class="gc-panel mt-3 overflow-x-auto px-4 py-4 text-sm leading-6 text-[var(--gc-ink)]">{rpcPayload}</pre>
						</div>
					</section>
				</div>
			</div>
		{/if}
	</div>
</div>
