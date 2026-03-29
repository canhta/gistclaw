<script lang="ts">
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
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
	const runs = $derived.by(() => {
		const clusters = work?.clusters ?? [];
		return clusters.flatMap((cluster) => [cluster.root, ...(cluster.children ?? [])]);
	});
	const modelUsage = $derived.by(() => {
		const counts: Record<string, number> = {};
		for (const run of runs) {
			const model = run.model_display?.trim();
			if (!model) {
				continue;
			}
			counts[model] = (counts[model] ?? 0) + 1;
		}
		return Object.entries(counts).map(([model, count]) => ({ model, count }));
	});
	const eventSources = $derived.by(() =>
		runs.map((run) => ({
			id: run.id,
			objective: run.objective,
			agentID: run.agent_id,
			streamURL: `/api/work/${encodeURIComponent(run.id)}/events`
		}))
	);

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
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Event stream handoff</h2>
				<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
					The raw event stream already exists per run, but GistClaw does not yet ship a
					section-level debug feed. Use this handoff to see which runs have SSE sources, then follow
					the richer operator timeline in Chat under the Run Events tab.
				</p>

				<div class="mt-6 grid gap-4 xl:grid-cols-3">
					<SurfaceMetricCard
						label="Live Sources"
						value={String(eventSources.length)}
						detail="Run-scoped SSE streams visible from the current queue."
					/>
					<SurfaceMetricCard
						label="Recovery Runs"
						value={String(work?.queue_strip.recovery_runs ?? 0)}
						detail="Runs that deserve event inspection first."
						tone="warning"
					/>
					<SurfaceMetricCard
						label="Operator Path"
						value="Chat -> Run Events"
						detail="Use Chat for decoded event timelines and live replay."
						tone="accent"
					/>
				</div>

				<div class="mt-6 grid gap-5 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,0.9fr)]">
					<section class="gc-panel px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">SSE sources</p>
						{#if eventSources.length === 0}
							<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
								No active run streams are visible right now.
							</p>
						{:else}
							<div class="mt-4 flex flex-col gap-4">
								{#each eventSources as source (source.id)}
									<div class="border-b border-[var(--gc-border)] pb-4 last:border-b-0 last:pb-0">
										<p class="gc-panel-title text-[var(--gc-ink)]">{source.objective}</p>
										<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
											{source.id} · {source.agentID}
										</p>
										<p class="gc-copy mt-3 font-mono text-sm break-all text-[var(--gc-signal)]">
											{source.streamURL}
										</p>
									</div>
								{/each}
							</div>
						{/if}
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Next step</p>
						<div class="mt-4 space-y-4">
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Chat</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Use Chat when you need the decoded operator timeline rather than raw SSE
									endpoints.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Run Events</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Run Events is the current supported place for streaming event playback and event
									kind inspection.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Logs</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Use Logs for connector-level output once the live tail backend arrives.
								</p>
							</div>
						</div>
					</section>
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
