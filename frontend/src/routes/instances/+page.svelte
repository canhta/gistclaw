<script lang="ts">
	import SurfaceLoadErrorPanel from '$lib/components/common/SurfaceLoadErrorPanel.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TabID = 'presence' | 'details';

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'presence', label: 'Presence' },
		{ id: 'details', label: 'Details' }
	];

	let activeTabOverride = $state<TabID | null>(null);

	function isTabID(value: string | null): value is TabID {
		return value === 'presence' || value === 'details';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'presence';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const instances = $derived(data.instances);
	const summary = $derived(instances?.summary ?? null);
	const lanes = $derived(instances?.lanes ?? []);
	const connectors = $derived(instances?.connectors ?? []);
	const sources = $derived(instances?.sources ?? null);
	const projectName = $derived(data.project?.active_name ?? 'No project');
	const projectPath = $derived(data.project?.active_path ?? 'No active project path');
	const instancesLoadError = $derived(data.instancesLoadError ?? '');
</script>

<svelte:head>
	<title>Instances | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Instances</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-6">
		{#if instances === null}
			{#if instancesLoadError !== ''}
				<SurfaceMessage label="INSTANCES" message={instancesLoadError} className="mb-4" />
			{/if}
			<div class="mt-2">
				<SurfaceLoadErrorPanel
					label="INSTANCES"
					title="Instances board unavailable"
					detail="The browser could not load the instance inventory feed from this daemon. Reload to retry."
				/>
			</div>
		{:else}
			<div class="grid gap-4 xl:grid-cols-4">
				<SurfaceMetricCard
					label="Front Lanes"
					value={String(summary?.front_lane_count ?? 0)}
					detail="Lead assistant lanes currently active, pending, or stopped on approval."
					tone="accent"
				/>
				<SurfaceMetricCard
					label="Specialist Lanes"
					value={String(summary?.specialist_lane_count ?? 0)}
					detail="Worker lanes derived from the visible work clusters."
					tone="accent"
				/>
				<SurfaceMetricCard
					label="Live Connectors"
					value={String(summary?.live_connector_count ?? 0)}
					detail="Connector lanes currently reporting active runtime state."
				/>
				<SurfaceMetricCard
					label="Pending Deliveries"
					value={String(summary?.pending_delivery_count ?? 0)}
					detail="Outbound messages still waiting on their next connector attempt."
					tone="warning"
				/>
				<SurfaceMetricCard label="Project" value={projectName} detail={projectPath} tone="accent" />
			</div>

			{#if activeTab === 'presence'}
				<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(18rem,0.75fr)]">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">PRESENCE</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Runtime presence board</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							OpenClaw treats presence as a live operator board. GistClaw now exposes a dedicated
							inventory feed that merges active work lanes with the connector beacons the daemon
							already tracks.
						</p>

						{#if lanes.length === 0 && connectors.length === 0}
							<div class="mt-5 border border-dashed border-[var(--gc-border)] px-4 py-5">
								<p class="gc-copy text-[var(--gc-ink)]">No active runtime presence</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
									Run work from Chat or connect a channel to populate this board.
								</p>
							</div>
						{:else}
							<div class="mt-5 grid gap-3">
								{#each lanes as lane (lane.id)}
									<article class="border border-[var(--gc-border)] px-4 py-4">
										<div class="flex flex-wrap items-start justify-between gap-4">
											<div>
												<p class="gc-stamp text-[var(--gc-ink-3)]">
													{lane.kind === 'front' ? 'Front lane' : 'Specialist lane'}
												</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink)]">{lane.agent_id}</p>
												<p class="gc-copy mt-1 text-[var(--gc-ink-2)]">{lane.objective}</p>
											</div>
											<span class="gc-stamp text-[var(--gc-primary)]">{lane.status_label}</span>
										</div>

										<div class="mt-4 grid gap-3 md:grid-cols-3">
											<div>
												<p class="gc-stamp text-[var(--gc-ink-3)]">Model</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink)]">
													{lane.model_display || 'Not recorded'}
												</p>
											</div>
											<div>
												<p class="gc-stamp text-[var(--gc-ink-3)]">Tokens</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink)]">
													{lane.token_summary || 'No token summary'}
												</p>
											</div>
											<div>
												<p class="gc-stamp text-[var(--gc-ink-3)]">Last activity</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink)]">
													{lane.last_activity_short || 'Unknown'}
												</p>
											</div>
										</div>
									</article>
								{/each}
							</div>
						{/if}
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">CONNECTOR BEACONS</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Connector beacons</h2>
						{#if connectors.length === 0}
							<p class="gc-copy mt-3 text-[var(--gc-ink-3)]">No live connector beacons.</p>
						{:else}
							<div class="mt-4 flex flex-col gap-3">
								{#each connectors as connector (connector.connector_id)}
									<div class="border border-[var(--gc-border)] px-4 py-4">
										<div class="flex items-center justify-between gap-4">
											<p class="gc-copy text-[var(--gc-ink)]">{connector.connector_id}</p>
											<span class="gc-stamp text-[var(--gc-primary)]">
												{connector.state_label}
											</span>
										</div>
										<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">{connector.summary}</p>
										<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
											{connector.pending_count} pending · {connector.retrying_count} retrying ·
											{connector.terminal_count} terminal
										</p>
									</div>
								{/each}
							</div>
						{/if}
					</section>
				</div>
			{:else}
				<div class="mt-6 grid gap-4 lg:grid-cols-2">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">DETAILS</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Source surfaces</h2>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							Instances stays honest about its inputs. Every value on this page comes through the
							instance inventory API, which is assembled from the shipped work, session, and
							connector health seams.
						</p>

						<div class="mt-5 grid gap-3 md:grid-cols-3">
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Work queue</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">{sources?.queue_headline}</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
									{sources?.root_runs ?? 0} root · {sources?.needs_approval_runs ?? 0} approvals
								</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Sessions</p>
								<p class="gc-panel-title mt-2 text-[var(--gc-ink)]">
									{sources?.session_count ?? 0}
								</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
									{sources?.connector_count ?? 0} connectors reporting through conversations
								</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Project</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">{projectName}</p>
								<p class="gc-copy mt-2 font-mono text-sm text-[var(--gc-ink-3)]">{projectPath}</p>
							</div>
						</div>
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">LINKED SURFACES</p>
						<div class="mt-3 grid gap-3">
							<div class="border border-[var(--gc-border)] px-4 py-3">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Chat</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-3">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Sessions</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-3">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Channels</p>
							</div>
						</div>
					</section>
				</div>
			{/if}
		{/if}
	</div>
</div>
