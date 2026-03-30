<script lang="ts">
	import { onDestroy } from 'svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import Composer from '$lib/components/chat/Composer.svelte';
	import TranscriptRow from '$lib/components/chat/TranscriptRow.svelte';
	import RunGraph from '$lib/components/graph/RunGraph.svelte';
	import { applyEvent, makeTranscriptState } from '$lib/chat/transcript.svelte';
	import { connectEventStream } from '$lib/http/events';
	import { requestJSON } from '$lib/http/client';
	import {
		loadLiveWorkSurface,
		type LiveWorkSurface,
		shouldRefreshWorkSurface
	} from '$lib/work/live';
	import type {
		WorkClusterResponse,
		WorkCreateResponse,
		WorkDetailResponse,
		WorkDismissResponse,
		WorkGraphNodeResponse,
		WorkNodeDetailResponse
	} from '$lib/types/api';
	import type { PageData } from './$types';

	type TabID = 'transcript' | 'graph' | 'run-events' | 'usage';

	let { data }: { data: PageData } = $props();

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'transcript', label: 'Transcript' },
		{ id: 'graph', label: 'Graph' },
		{ id: 'run-events', label: 'Run Events' },
		{ id: 'usage', label: 'Usage' }
	];

	let activeTabOverride = $state<TabID | null>(null);
	let selectedRunId = $state<string | null>(null);
	let detail = $state<WorkDetailResponse | null>(null);
	let nodeDetail = $state<WorkNodeDetailResponse | null>(null);
	let selectedNodeID = $state<string | null>(null);
	let detailLoading = $state(false);
	let nodeDetailLoading = $state(false);
	let detailError = $state('');
	let connectedRunId = $state<string | null>(null);
	let refreshTimer: ReturnType<typeof setTimeout> | null = null;
	let refreshRequestID = 0;

	const transcript = makeTranscriptState();
	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'transcript';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const queue = $derived(data.chat?.queue);
	const runs: WorkClusterResponse[] = $derived(data.chat?.runs ?? []);
	const paging = $derived(data.chat?.paging);
	const activeDetail = $derived(detail ?? data.chat?.detail ?? null);
	const activeNodeDetail = $derived(nodeDetail ?? data.chat?.nodeDetail ?? null);
	const activeSelectedNodeID = $derived(
		selectedNodeID ??
			data.chat?.inspectorNodeID ??
			data.chat?.nodeDetail?.id ??
			data.chat?.detail?.inspector_seed?.id ??
			null
	);

	let stopStream: (() => void) | null = null;
	let rawEvents = $state<Array<{ kind: string; occurred_at: string }>>([]);
	let actionError = $state('');
	let streamError = $state('');

	function effectiveRunStatus(): 'idle' | 'active' | 'completed' | 'failed' | 'interrupted' {
		if (transcript.runStatus !== 'idle') {
			return transcript.runStatus;
		}
		switch (activeDetail?.run.status) {
			case 'active':
			case 'pending':
			case 'needs_approval':
				return 'active';
			case 'completed':
				return 'completed';
			case 'failed':
				return 'failed';
			case 'interrupted':
				return 'interrupted';
			default:
				return 'idle';
		}
	}

	const composerRunStatus = $derived.by(() => effectiveRunStatus());
	const canInject = $derived.by(() => {
		if (!selectedRunId) {
			return false;
		}
		const status = activeDetail?.run.status ?? '';
		return status === 'active' || status === 'pending' || status === 'needs_approval';
	});
	const usageTotals = $derived.by(() => {
		const input = activeDetail?.run.input_tokens ?? transcript.tokenSummary.inputTokens;
		const output = activeDetail?.run.output_tokens ?? transcript.tokenSummary.outputTokens;
		return {
			input,
			output,
			total: activeDetail?.run.total_tokens ?? input + output
		};
	});
	const usageLedger = $derived.by(() =>
		[...(activeDetail?.graph.nodes ?? [])]
			.filter((node) => usageNodeTotal(node) > 0 || node.token_summary.trim() !== '')
			.sort((left, right) => {
				if (left.is_root !== right.is_root) {
					return left.is_root ? -1 : 1;
				}
				if (left.is_active_path !== right.is_active_path) {
					return left.is_active_path ? -1 : 1;
				}
				const totalDelta = usageNodeTotal(right) - usageNodeTotal(left);
				if (totalDelta !== 0) {
					return totalDelta;
				}
				return left.agent_id.localeCompare(right.agent_id);
			})
	);

	function isTabID(value: string | null): value is TabID {
		return (
			value === 'transcript' || value === 'graph' || value === 'run-events' || value === 'usage'
		);
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	function formatUsageCount(value: number | undefined): string {
		return (value ?? 0).toLocaleString();
	}

	function usageNodeTotal(node: WorkGraphNodeResponse): number {
		if (typeof node.total_tokens === 'number') {
			return node.total_tokens;
		}
		return (node.input_tokens ?? 0) + (node.output_tokens ?? 0);
	}

	function usageNodeObjective(node: WorkGraphNodeResponse): string {
		const preview = node.objective_preview.trim();
		if (preview !== '') {
			return preview;
		}
		const objective = node.objective.trim();
		if (objective !== '') {
			return objective;
		}
		return 'No task text saved.';
	}

	function connectToRun(runId: string, streamURL: string): void {
		stopStream?.();
		stopStream = null;
		transcript.reset();
		rawEvents = [];
		streamError = '';
		selectedRunId = runId;
		connectedRunId = runId;

		stopStream = connectEventStream(
			streamURL,
			(delta) => {
				applyEvent(transcript, delta);
				rawEvents.push({ kind: delta.kind, occurred_at: delta.occurred_at });
				if (shouldRefreshWorkSurface(delta.kind)) {
					scheduleLiveSurfaceRefresh(runId);
				}
			},
			() => {
				streamError = 'Stream disconnected.';
			}
		);
	}

	function clearRefreshTimer(): void {
		if (!refreshTimer) {
			return;
		}
		clearTimeout(refreshTimer);
		refreshTimer = null;
	}

	async function refreshLiveSurface(
		runId: string,
		requestedNodeID = activeSelectedNodeID ?? ''
	): Promise<void> {
		const requestID = ++refreshRequestID;
		const nextSurface = await loadLiveWorkSurface(
			globalThis.fetch.bind(globalThis),
			runId,
			requestedNodeID
		);
		if (selectedRunId !== runId || requestID !== refreshRequestID) {
			return;
		}
		detail = nextSurface.detail;
		nodeDetail = nextSurface.nodeDetail;
		selectedNodeID = nextSurface.inspectorNodeID;
	}

	function scheduleLiveSurfaceRefresh(runId: string): void {
		clearRefreshTimer();
		refreshTimer = setTimeout(() => {
			refreshTimer = null;
			void refreshLiveSurface(runId).catch(() => {
				// Keep the graph surface stable until the next event arrives.
			});
		}, 120);
	}

	function currentSeededSurface(runId: string): LiveWorkSurface | null {
		const seededDetail = detail?.run.id === runId ? detail : (data.chat?.detail ?? null);
		if (!seededDetail || seededDetail.run.id !== runId) {
			return null;
		}
		return {
			detail: seededDetail,
			nodeDetail:
				data.chat?.detail?.run.id === runId
					? (data.chat?.nodeDetail ?? null)
					: (nodeDetail ?? null),
			inspectorNodeID:
				data.chat?.detail?.run.id === runId
					? (data.chat?.inspectorNodeID ?? data.chat?.nodeDetail?.id ?? null)
					: (selectedNodeID ?? null)
		};
	}

	async function selectRun(runId: string, seededSurface?: LiveWorkSurface | null): Promise<void> {
		if (connectedRunId === runId && selectedRunId === runId) {
			return;
		}

		detailLoading = true;
		detailError = '';
		selectedRunId = runId;

		try {
			const nextSurface =
				seededSurface ??
				(await loadLiveWorkSurface(
					globalThis.fetch.bind(globalThis),
					runId,
					activeSelectedNodeID ?? ''
				));
			detail = nextSurface.detail;
			nodeDetail = nextSurface.nodeDetail;
			selectedNodeID = nextSurface.inspectorNodeID;
			nodeDetailLoading = false;
			connectToRun(runId, nextSurface.detail.run.stream_url);
		} catch {
			detail = seededSurface?.detail ?? null;
			nodeDetail = seededSurface?.nodeDetail ?? null;
			selectedNodeID = seededSurface?.inspectorNodeID ?? null;
			nodeDetailLoading = false;
			detailError = 'Failed to load run detail.';
		} finally {
			detailLoading = false;
		}
	}

	$effect(() => {
		selectedRunId = data.chat?.selectedRunID ?? null;
		detail = data.chat?.detail ?? null;
		nodeDetail = data.chat?.nodeDetail ?? null;
		selectedNodeID = data.chat?.inspectorNodeID ?? data.chat?.nodeDetail?.id ?? null;
		nodeDetailLoading = false;
		detailError = '';
		connectedRunId = null;
		clearRefreshTimer();
	});

	$effect(() => {
		if (!selectedRunId) {
			return;
		}
		if (connectedRunId === selectedRunId) {
			return;
		}

		void selectRun(selectedRunId, currentSeededSurface(selectedRunId));
	});

	async function handleGraphNodeSelect(nodeID: string): Promise<void> {
		if (!selectedRunId || nodeID.trim() === '') {
			return;
		}
		if (nodeID === selectedNodeID && nodeDetail?.id === nodeID) {
			return;
		}

		selectedNodeID = nodeID;
		nodeDetail = nodeDetail?.id === nodeID ? nodeDetail : null;
		nodeDetailLoading = true;

		try {
			await refreshLiveSurface(selectedRunId, nodeID);
		} catch {
			nodeDetail = null;
		} finally {
			nodeDetailLoading = false;
		}
	}

	async function handleSend(text: string): Promise<void> {
		actionError = '';
		try {
			const result = await requestJSON<WorkCreateResponse>(fetch, '/api/work', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ task: text })
			});
			await selectRun(result.run_id);
		} catch {
			actionError = 'Failed to send message. Please try again.';
		}
	}

	async function handleInject(text: string): Promise<void> {
		if (!selectedRunId) {
			return;
		}
		actionError = '';
		try {
			await requestJSON<{ injected: boolean; run_id: string; message_id: string; kind: string }>(
				fetch,
				`/api/work/${selectedRunId}/inject`,
				{
					method: 'POST',
					headers: { 'content-type': 'application/json' },
					body: JSON.stringify({ note: text })
				}
			);
		} catch {
			actionError = 'Failed to inject note. Please try again.';
		}
	}

	async function handleStop(): Promise<void> {
		if (!selectedRunId) return;
		try {
			await requestJSON<WorkDismissResponse>(fetch, `/api/work/${selectedRunId}/dismiss`, {
				method: 'POST'
			});
		} catch {
			// Stream will reflect the interrupted state
		}
	}

	onDestroy(() => {
		clearRefreshTimer();
		stopStream?.();
	});
</script>

<svelte:head>
	<title>Chat | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="flex items-center justify-between px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Chat</h1>
			{#if activeDetail?.run.status_label}
				<span class="gc-badge border-[var(--gc-primary)] text-[var(--gc-primary)]">
					{activeDetail.run.status_label}
				</span>
			{:else if transcript.runStatus === 'active'}
				<span class="gc-badge border-[var(--gc-primary)] text-[var(--gc-primary)]">ACTIVE</span>
			{:else if transcript.runStatus === 'interrupted'}
				<span class="gc-badge border-[var(--gc-warning)] text-[var(--gc-warning)]">INTERRUPTED</span
				>
			{:else if transcript.runStatus === 'failed'}
				<span class="gc-badge border-[var(--gc-error)] text-[var(--gc-error)]">FAILED</span>
			{/if}
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-6">
		<div class="grid gap-4 xl:grid-cols-4">
			<SurfaceMetricCard
				label="Root Runs"
				value={String(queue?.root_runs ?? 0)}
				detail={queue?.headline ?? 'No active runs'}
			/>
			<SurfaceMetricCard
				label="Worker Runs"
				value={String(queue?.worker_runs ?? 0)}
				detail="Specialist runs currently attached to visible root work."
			/>
			<SurfaceMetricCard
				label="Recovery Runs"
				value={String(queue?.recovery_runs ?? 0)}
				detail="Runs that need recovery or operator intervention."
				tone="warning"
			/>
			<SurfaceMetricCard
				label="Project"
				value={data.chat?.projectName || 'No project'}
				detail={data.chat?.projectPath || 'No active project path'}
				tone="accent"
			/>
		</div>

		<section class="gc-panel-soft mt-5 px-5 py-5">
			<p class="gc-stamp text-[var(--gc-ink-3)]">RUN HEADER</p>
			{#if detailLoading}
				<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">Loading run detail…</p>
			{:else if detailError}
				<p class="gc-copy mt-3 text-[var(--gc-error)]">{detailError}</p>
			{:else if activeDetail}
				<div class="flex flex-wrap items-start justify-between gap-4">
					<div class="min-w-0 flex-1">
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
							{activeDetail.run.objective_text}
						</h2>
						<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
							{activeDetail.run.trigger_label} · {activeDetail.run.state_label}
						</p>
					</div>
					<span class="gc-badge border-[var(--gc-border-strong)] text-[var(--gc-ink-2)]">
						{activeDetail.run.short_id}
					</span>
				</div>

				<div class="mt-5 grid gap-4 sm:grid-cols-2 xl:grid-cols-6">
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">Started</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">{activeDetail.run.started_at_label}</p>
					</div>
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">Last Activity</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">{activeDetail.run.last_activity_label}</p>
					</div>
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">Model</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">{activeDetail.run.model_display}</p>
					</div>
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">Tokens</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">{activeDetail.run.token_summary}</p>
					</div>
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">Turns</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">{activeDetail.run.turn_count}</p>
					</div>
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">Events</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">{activeDetail.run.event_count}</p>
					</div>
				</div>
			{:else}
				<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">No run selected.</p>
			{/if}
		</section>

		<div class="mt-5 flex min-h-0 flex-1 flex-col">
			{#if activeTab === 'transcript'}
				<div class="flex min-h-0 flex-1 flex-col">
					<div
						class="flex-1 overflow-y-auto border border-[var(--gc-border)] bg-[var(--gc-surface)]"
					>
						{#if actionError}
							<div
								class="border-b border-b-[1.5px] border-[var(--gc-error)] bg-[var(--gc-error-dim)] px-5 py-3"
							>
								<p class="gc-stamp text-[var(--gc-error)]">Action error</p>
								<p class="gc-copy mt-1 text-[var(--gc-ink)]">{actionError}</p>
							</div>
						{/if}
						{#if streamError}
							<div
								class="border-b border-b-[1.5px] border-[var(--gc-warning)] bg-[var(--gc-warning-dim)] px-5 py-3"
							>
								<p class="gc-stamp text-[var(--gc-warning)]">Connection</p>
								<p class="gc-copy mt-1 text-[var(--gc-ink)]">{streamError}</p>
							</div>
						{/if}

						{#if transcript.rows.length === 0}
							<div class="flex h-full items-center justify-center p-10">
								<div class="text-center">
									<p class="gc-stamp text-[var(--gc-ink-3)]">CHAT</p>
									<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No active session</p>
									<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
										Start a conversation to begin. Type a message below to run your first task.
									</p>
								</div>
							</div>
						{:else}
							{#each transcript.rows as row (row.id)}
								<TranscriptRow {row} />
							{/each}
						{/if}
					</div>

					<Composer
						runStatus={composerRunStatus}
						{canInject}
						onSend={handleSend}
						onInject={handleInject}
						onStop={handleStop}
					/>
				</div>
			{:else if activeTab === 'graph'}
				{#if activeDetail?.graph}
					<RunGraph
						graph={activeDetail.graph}
						inspectorSeedID={activeDetail.inspector_seed?.id}
						nodeDetail={activeNodeDetail}
						{nodeDetailLoading}
						selectedNodeID={activeSelectedNodeID}
						onselectnode={(nodeID) => void handleGraphNodeSelect(nodeID)}
					/>
				{:else}
					<div class="flex flex-1 items-center justify-center p-10">
						<div class="text-center">
							<p class="gc-stamp text-[var(--gc-ink-3)]">GRAPH</p>
							<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No run graph selected</p>
							<p class="gc-copy mt-3 max-w-sm text-[var(--gc-ink-2)]">
								Choose a run from the active queue to inspect its orchestration graph and active
								path.
							</p>
						</div>
					</div>
				{/if}
			{:else if activeTab === 'run-events'}
				<div class="gc-panel flex-1 overflow-y-auto border-[var(--gc-border)] px-5 py-4">
					{#if runs.length === 0}
						<p class="gc-copy text-[var(--gc-ink-3)]">No runs yet.</p>
					{:else}
						<div class="mb-4 flex flex-wrap gap-2">
							{#each runs as cluster (cluster.root.id)}
								<button
									onclick={() => void selectRun(cluster.root.id)}
									class="gc-action px-3 py-1 text-[10px] {selectedRunId === cluster.root.id
										? 'border-[var(--gc-primary)] text-[var(--gc-primary)]'
										: 'text-[var(--gc-ink-3)]'}"
								>
									{cluster.root.id.slice(0, 6)}
								</button>
							{/each}
						</div>
						{#if rawEvents.length === 0}
							<p class="gc-copy text-[var(--gc-ink-3)]">Waiting for events…</p>
						{:else}
							<div class="gc-panel border-[var(--gc-border)]">
								{#each rawEvents as e, i (i)}
									<div
										class="flex items-center gap-4 border-b border-b-[1px] border-[var(--gc-border)] px-3 py-2"
									>
										<span class="gc-stamp w-48 shrink-0 text-[var(--gc-ink-2)]">{e.kind}</span>
										<time class="gc-machine text-[var(--gc-ink-3)]">{e.occurred_at}</time>
									</div>
								{/each}
							</div>
						{/if}
					{/if}
				</div>
			{:else if activeDetail}
				<div class="grid flex-1 gap-4 xl:grid-cols-[minmax(0,1.05fr)_minmax(18rem,0.85fr)]">
					<section class="gc-panel flex-1 overflow-y-auto border-[var(--gc-border)] px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Usage board</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Run token usage</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							Current totals and node-by-node token posture from the selected run graph.
						</p>

						<div class="mt-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Total tokens</p>
								<p class="gc-value mt-2 text-[1.2rem]">{formatUsageCount(usageTotals.total)}</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">{activeDetail.run.token_summary}</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Input</p>
								<p class="gc-value mt-2 text-[1.2rem]">{formatUsageCount(usageTotals.input)}</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">Prompt and context tokens</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Output</p>
								<p class="gc-value mt-2 text-[1.2rem]">{formatUsageCount(usageTotals.output)}</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">Assistant response tokens</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Active path</p>
								<p class="gc-value mt-2 text-[1.2rem]">{activeDetail.graph.active_path.length}</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">Nodes currently on the live path</p>
							</div>
						</div>

						<div class="mt-6">
							<div class="flex flex-wrap items-center justify-between gap-3">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Node ledger</p>
								<p class="gc-copy text-[var(--gc-ink-3)]">{usageLedger.length} nodes visible</p>
							</div>

							{#if usageLedger.length === 0}
								<div class="mt-4 border border-[var(--gc-border)] px-4 py-4">
									<p class="gc-copy text-[var(--gc-ink-2)]">
										This run has not reported token usage from any graph node yet.
									</p>
								</div>
							{:else}
								<div class="mt-4 grid gap-3">
									{#each usageLedger as node (node.id)}
										<article class="border border-[var(--gc-border)] px-4 py-4">
											<div class="flex flex-wrap items-start justify-between gap-4">
												<div class="min-w-0 flex-1">
													<div class="flex flex-wrap items-center gap-2">
														<p class="gc-panel-title text-[var(--gc-ink)]">{node.agent_id}</p>
														{#if node.is_active_path}
															<span
																class="gc-badge border-[var(--gc-signal)] text-[var(--gc-signal)]"
															>
																ACTIVE PATH
															</span>
														{/if}
														{#if activeNodeDetail?.id === node.id}
															<span
																class="gc-badge border-[var(--gc-primary)] text-[var(--gc-primary)]"
															>
																FOCUSED
															</span>
														{/if}
													</div>
													<p class="gc-copy mt-2 text-[var(--gc-ink)]">
														{usageNodeObjective(node)}
													</p>
													<div class="mt-3 flex flex-wrap gap-2">
														<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
															{node.status_label}
														</span>
														<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
															{node.model_display}
														</span>
													</div>
												</div>

												<div class="text-right">
													<p class="gc-value text-[1.2rem]">
														{formatUsageCount(usageNodeTotal(node))}
													</p>
													<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">{node.token_summary}</p>
												</div>
											</div>
										</article>
									{/each}
								</div>
							{/if}
						</div>
					</section>

					<aside class="grid gap-4">
						<section class="gc-panel-soft px-5 py-5">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Focused node</p>
							{#if activeNodeDetail}
								<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">{activeNodeDetail.agent_id}</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									{activeNodeDetail.task.preview_text ||
										activeNodeDetail.task.plain_text ||
										'No task text saved.'}
								</p>
								<div class="mt-4 grid gap-3">
									<div class="border border-[var(--gc-border)] px-4 py-4">
										<p class="gc-stamp text-[var(--gc-ink-3)]">Exact usage</p>
										<p class="gc-copy mt-2 text-[var(--gc-ink)]">
											{activeNodeDetail.token_exact_summary}
										</p>
									</div>
									<div class="border border-[var(--gc-border)] px-4 py-4">
										<p class="gc-stamp text-[var(--gc-ink-3)]">Summary</p>
										<p class="gc-copy mt-2 text-[var(--gc-ink)]">
											{activeNodeDetail.token_summary}
										</p>
									</div>
									<div class="border border-[var(--gc-border)] px-4 py-4">
										<p class="gc-stamp text-[var(--gc-ink-3)]">Status</p>
										<p class="gc-copy mt-2 text-[var(--gc-ink)]">
											{activeNodeDetail.status_label}
										</p>
									</div>
								</div>
							{:else}
								<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
									Select a graph node to inspect exact per-node usage.
								</p>
							{/if}
						</section>

						<section class="gc-panel-soft px-5 py-5">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Coverage</p>
							<div class="mt-4 grid gap-3">
								<div class="border border-[var(--gc-border)] px-4 py-4">
									<p class="gc-stamp text-[var(--gc-ink-3)]">Nodes with usage</p>
									<p class="gc-value mt-2 text-[1.1rem]">{usageLedger.length}</p>
								</div>
								<div class="border border-[var(--gc-border)] px-4 py-4">
									<p class="gc-stamp text-[var(--gc-ink-3)]">Run model</p>
									<p class="gc-copy mt-2 text-[var(--gc-ink)]">{activeDetail.run.model_display}</p>
								</div>
								<div class="border border-[var(--gc-border)] px-4 py-4">
									<p class="gc-stamp text-[var(--gc-ink-3)]">Last activity</p>
									<p class="gc-copy mt-2 text-[var(--gc-ink)]">
										{activeDetail.run.last_activity_label}
									</p>
								</div>
							</div>
						</section>
					</aside>
				</div>
			{:else}
				<div class="flex flex-1 items-center justify-center p-10">
					<div class="text-center">
						<p class="gc-stamp text-[var(--gc-ink-3)]">USAGE</p>
						<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No run selected</p>
						<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
							Choose a run from the queue to inspect token usage and node-level totals.
						</p>
					</div>
				</div>
			{/if}
		</div>

		{#if runs.length > 0}
			<div
				class="mt-5 shrink-0 border-t border-t-[1.5px] border-[var(--gc-border)] bg-[var(--gc-surface)]"
			>
				<div class="flex gap-2 overflow-x-auto px-4 py-2">
					{#each runs as cluster (cluster.root.id)}
						<button
							onclick={() => void selectRun(cluster.root.id)}
							title={cluster.root.objective}
							class="gc-panel-soft flex max-w-[200px] shrink-0 flex-col px-3 py-2 text-left transition-colors hover:border-[var(--gc-border-strong)] {selectedRunId ===
							cluster.root.id
								? 'border-[var(--gc-primary)]'
								: ''}"
						>
							<span class="gc-stamp text-[var(--gc-ink-3)]">{cluster.root.status_label}</span>
							<span class="gc-copy mt-1 truncate text-[var(--gc-ink-2)]"
								>{cluster.root.objective}</span
							>
						</button>
					{/each}
				</div>
			</div>
		{/if}

		{#if paging?.has_prev || paging?.has_next}
			<div class="mt-5 flex items-center justify-between border-t border-[var(--gc-border)] pt-4">
				<p class="gc-copy text-[var(--gc-ink-2)]">
					Page through older and newer root runs in the active project.
				</p>
				<div class="flex gap-3">
					{#if paging?.prevHref}
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
						<a href={paging.prevHref} class="gc-action gc-action-accent">Previous Page</a>
					{/if}
					{#if paging?.nextHref}
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
						<a href={paging.nextHref} class="gc-action gc-action-solid">Next Page</a>
					{/if}
				</div>
			</div>
		{/if}
	</div>
</div>
