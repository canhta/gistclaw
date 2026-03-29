<script lang="ts">
	import { onDestroy } from 'svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import Composer from '$lib/components/chat/Composer.svelte';
	import TranscriptRow from '$lib/components/chat/TranscriptRow.svelte';
	import { applyEvent, makeTranscriptState } from '$lib/chat/transcript.svelte';
	import { connectEventStream } from '$lib/http/events';
	import { requestJSON } from '$lib/http/client';
	import { loadWorkDetail } from '$lib/work/load';
	import type {
		WorkClusterResponse,
		WorkCreateResponse,
		WorkDetailResponse,
		WorkDismissResponse
	} from '$lib/types/api';
	import type { PageData } from './$types';

	type TabID = 'transcript' | 'run-events' | 'usage';

	let { data }: { data: PageData } = $props();

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'transcript', label: 'Transcript' },
		{ id: 'run-events', label: 'Run Events' },
		{ id: 'usage', label: 'Usage' }
	];

	let activeTabOverride = $state<TabID | null>(null);
	let selectedRunId = $state<string | null>(null);
	let detail = $state<WorkDetailResponse | null>(null);
	let detailLoading = $state(false);
	let detailError = $state('');
	let connectedRunId = $state<string | null>(null);

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

	let stopStream: (() => void) | null = null;
	let rawEvents = $state<Array<{ kind: string; occurred_at: string }>>([]);
	let sendError = $state('');
	let streamError = $state('');

	function isTabID(value: string | null): value is TabID {
		return value === 'transcript' || value === 'run-events' || value === 'usage';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
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
			},
			() => {
				streamError = 'Stream disconnected.';
			}
		);
	}

	async function selectRun(runId: string, seededDetail?: WorkDetailResponse | null): Promise<void> {
		if (connectedRunId === runId && selectedRunId === runId) {
			return;
		}

		detailLoading = true;
		detailError = '';
		selectedRunId = runId;

		try {
			const nextDetail =
				seededDetail ?? (await loadWorkDetail(globalThis.fetch.bind(globalThis), runId));
			detail = nextDetail;
			connectToRun(runId, nextDetail.run.stream_url);
		} catch {
			detail = seededDetail ?? null;
			detailError = 'Failed to load run detail.';
		} finally {
			detailLoading = false;
		}
	}

	$effect(() => {
		selectedRunId = data.chat?.selectedRunID ?? null;
		detail = data.chat?.detail ?? null;
		detailError = '';
		connectedRunId = null;
	});

	$effect(() => {
		if (!selectedRunId) {
			return;
		}
		if (connectedRunId === selectedRunId) {
			return;
		}

		void selectRun(
			selectedRunId,
			detail?.run.id === selectedRunId ? detail : (data.chat?.detail ?? null)
		);
	});

	async function handleSend(text: string): Promise<void> {
		sendError = '';
		try {
			const result = await requestJSON<WorkCreateResponse>(fetch, '/api/work', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ task: text })
			});
			await selectRun(result.run_id);
		} catch {
			sendError = 'Failed to send message. Please try again.';
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
						{#if sendError}
							<div
								class="border-b border-b-[1.5px] border-[var(--gc-error)] bg-[var(--gc-error-dim)] px-5 py-3"
							>
								<p class="gc-stamp text-[var(--gc-error)]">Send error</p>
								<p class="gc-copy mt-1 text-[var(--gc-ink)]">{sendError}</p>
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

					<Composer runStatus={transcript.runStatus} onSend={handleSend} onStop={handleStop} />
				</div>
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
			{:else}
				<div class="flex flex-1 items-center justify-center p-10">
					{#if transcript.tokenSummary.inputTokens > 0 || transcript.tokenSummary.outputTokens > 0}
						<div class="gc-panel max-w-sm px-6 py-5 text-center">
							<p class="gc-stamp text-[var(--gc-ink-3)]">TOKEN USAGE</p>
							<div class="mt-4 grid grid-cols-2 gap-4">
								<div>
									<p class="gc-stamp text-[var(--gc-ink-3)]">Input</p>
									<p class="gc-panel-title mt-1 text-[var(--gc-ink)]">
										{transcript.tokenSummary.inputTokens.toLocaleString()}
									</p>
								</div>
								<div>
									<p class="gc-stamp text-[var(--gc-ink-3)]">Output</p>
									<p class="gc-panel-title mt-1 text-[var(--gc-ink)]">
										{transcript.tokenSummary.outputTokens.toLocaleString()}
									</p>
								</div>
							</div>
						</div>
					{:else}
						<div class="text-center">
							<p class="gc-stamp text-[var(--gc-ink-3)]">USAGE</p>
							<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No usage data</p>
							<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
								Token usage will appear here after a run completes.
							</p>
						</div>
					{/if}
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
