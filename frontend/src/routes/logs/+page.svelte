<script lang="ts">
	import { resolve } from '$app/paths';
	import SurfaceLoadErrorPanel from '$lib/components/common/SurfaceLoadErrorPanel.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import { connectLogStream } from '$lib/logs/events';
	import type { LogEntryResponse } from '$lib/types/api';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TabID = 'live-tail' | 'filters' | 'export';
	type TailState = 'idle' | 'connecting' | 'live' | 'error';

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'live-tail', label: 'Live Tail' },
		{ id: 'filters', label: 'Filters' },
		{ id: 'export', label: 'Export' }
	];
	const limitOptions = [50, 100, 200, 500];

	let activeTabOverride = $state<TabID | null>(null);
	let tailPaused = $state(false);
	let autoFollow = $state(true);
	let tailState = $state<TailState>('idle');
	let streamError = $state('');
	let exportNotice = $state('');
	let outputPane = $state<HTMLDivElement | null>(null);
	let entries = $state<LogEntryResponse[]>([]);
	let bufferedEntries = $state(0);

	function isTabID(value: string | null): value is TabID {
		return value === 'live-tail' || value === 'filters' || value === 'export';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	function trimEntries(items: LogEntryResponse[], limit: number): LogEntryResponse[] {
		if (items.length <= limit) {
			return items;
		}
		return items.slice(items.length - limit);
	}

	function filterTabQuery(): string {
		const parts = ['tab=filters'];
		if (!logs) {
			return parts.join('&');
		}
		if (logs.filters.query !== '') {
			parts.push(`q=${encodeURIComponent(logs.filters.query)}`);
		}
		if (logs.filters.level !== 'all') {
			parts.push(`level=${encodeURIComponent(logs.filters.level)}`);
		}
		if (logs.filters.source !== 'all') {
			parts.push(`source=${encodeURIComponent(logs.filters.source)}`);
		}
		parts.push(`limit=${encodeURIComponent(String(logs.filters.limit))}`);
		return parts.join('&');
	}

	async function copyJSONL(): Promise<void> {
		if (typeof navigator === 'undefined' || !navigator.clipboard) {
			exportNotice = 'Clipboard is unavailable in this browser.';
			return;
		}
		try {
			await navigator.clipboard.writeText(exportJSONL);
			exportNotice = 'Copied current buffer.';
		} catch {
			exportNotice = 'Failed to copy the current buffer.';
		}
	}

	function downloadJSONL(): void {
		if (typeof document === 'undefined') {
			return;
		}
		const blob = new Blob([exportJSONL], { type: 'application/x-ndjson;charset=utf-8' });
		const href = URL.createObjectURL(blob);
		const link = document.createElement('a');
		link.href = href;
		link.download = 'gistclaw-logs.jsonl';
		link.click();
		URL.revokeObjectURL(href);
		exportNotice = 'Downloaded current buffer.';
	}

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'live-tail';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const projectName = $derived(data.project?.active_name ?? 'No project');
	const projectPath = $derived(data.project?.active_path ?? 'No active project path');
	const logs = $derived(data.logs);
	const logsNotice = $derived(data.logsLoadError);
	const filterSourceOptions = $derived.by(() => [
		'all',
		...(logs?.sources.filter((source) => source !== 'all') ?? [])
	]);
	const renderedEntries = $derived(
		entries.length === 0 && (logs?.entries.length ?? 0) > 0 ? (logs?.entries ?? []) : entries
	);
	const renderedBufferedEntries = $derived(
		bufferedEntries === 0 && (logs?.summary.buffered_entries ?? 0) > 0
			? (logs?.summary.buffered_entries ?? 0)
			: bufferedEntries
	);
	const errorEntries = $derived(renderedEntries.filter((entry) => entry.level === 'error').length);
	const warningEntries = $derived(renderedEntries.filter((entry) => entry.level === 'warn').length);
	const tailStatusValue = $derived.by(() => {
		if (tailPaused) {
			return 'Paused';
		}
		switch (tailState) {
			case 'live':
				return 'Live';
			case 'connecting':
				return 'Connecting';
			case 'error':
				return 'Degraded';
			default:
				return 'Ready';
		}
	});
	const tailStatusDetail = $derived.by(() => {
		if (streamError !== '') {
			return streamError;
		}
		if (tailPaused) {
			return 'Live stream is paused. Resume to keep following new lines.';
		}
		return 'The current process buffer is available through the live stream endpoint.';
	});
	const exportJSONL = $derived.by(() =>
		renderedEntries
			.map((entry) =>
				JSON.stringify({
					id: entry.id,
					source: entry.source,
					level: entry.level,
					level_label: entry.level_label,
					message: entry.message,
					raw: entry.raw,
					created_at_label: entry.created_at_label
				})
			)
			.join('\n')
	);
	const exportCountLabel = $derived.by(() =>
		renderedEntries.length === 1
			? '1 entry ready to hand off.'
			: `${renderedEntries.length} entries ready to hand off.`
	);

	$effect(() => {
		if (!logs) {
			entries = [];
			bufferedEntries = 0;
			streamError = '';
			exportNotice = '';
			return;
		}
		entries = [...logs.entries];
		bufferedEntries = logs.summary.buffered_entries;
		streamError = '';
		exportNotice = '';
	});

	$effect(() => {
		if (!logs || activeTab !== 'live-tail' || tailPaused) {
			tailState = 'idle';
			return;
		}

		tailState = 'connecting';
		const disconnect = connectLogStream(
			logs.stream_url,
			(entry) => {
				bufferedEntries += 1;
				entries = trimEntries([...entries, entry], logs.filters.limit);
				tailState = 'live';
				streamError = '';
				if (autoFollow && outputPane) {
					requestAnimationFrame(() => {
						outputPane?.scrollTo({ top: outputPane.scrollHeight });
					});
				}
			},
			() => {
				tailState = 'error';
				streamError = 'Tail disconnected. Resume to retry the stream.';
			}
		);

		return () => {
			disconnect();
		};
	});
</script>

<svelte:head>
	<title>Logs | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Logs</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-6">
		{#if logs === null}
			{#if logsNotice !== ''}
				<SurfaceMessage label="LOGS" message={logsNotice} className="mb-4" />
			{/if}
			<div class="mt-2">
				<SurfaceLoadErrorPanel
					label="LOGS"
					title="Logs board unavailable"
					detail="The browser could not load the runtime log feed from this daemon. Reload to retry."
				/>
			</div>
		{:else}
			<div class="grid gap-4 xl:grid-cols-4">
				<SurfaceMetricCard
					label="Tail Status"
					value={tailStatusValue}
					detail={tailStatusDetail}
					tone={tailState === 'error' ? 'warning' : tailState === 'live' ? 'accent' : 'default'}
				/>
				<SurfaceMetricCard
					label="Buffered Lines"
					value={String(renderedBufferedEntries)}
					detail="Process-local lines currently retained by the daemon."
				/>
				<SurfaceMetricCard
					label="Visible Window"
					value={String(renderedEntries.length)}
					detail={`Filtered window capped at ${logs.filters.limit} lines.`}
				/>
				<SurfaceMetricCard label="Project" value={projectName} detail={projectPath} tone="accent" />
			</div>

			{#if activeTab === 'live-tail'}
				<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.45fr)_minmax(18rem,0.8fr)]">
					<section class="gc-panel-soft px-5 py-5">
						<div class="flex flex-wrap items-start justify-between gap-4">
							<div>
								<p class="gc-stamp text-[var(--gc-ink-3)]">LIVE TAIL</p>
								<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Live tail</h2>
								<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
									Current process output, filtered through the active query and kept pinned to the
									most recent lines when auto-follow is on.
								</p>
							</div>

							<div class="flex flex-wrap gap-3">
								<button
									type="button"
									onclick={() => {
										tailPaused = !tailPaused;
										streamError = '';
									}}
									class={`gc-action px-4 py-2 ${tailPaused ? 'gc-action-solid' : ''}`}
								>
									{tailPaused ? 'Resume Tail' : 'Pause Tail'}
								</button>
								<button
									type="button"
									onclick={() => (autoFollow = !autoFollow)}
									class={`gc-action px-4 py-2 ${autoFollow ? 'gc-action-solid' : ''}`}
									aria-pressed={autoFollow}
								>
									Auto-follow
								</button>
								<a href={resolve(`/logs?${filterTabQuery()}`)} class="gc-action px-4 py-2">
									Review Filters
								</a>
							</div>
						</div>

						<div
							bind:this={outputPane}
							class="mt-5 max-h-[34rem] overflow-y-auto border border-[var(--gc-border)] bg-[var(--gc-canvas)] px-4 py-4"
						>
							<div
								class="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--gc-border)] pb-3"
							>
								<p class="gc-stamp text-[var(--gc-ink-3)]">Runtime output</p>
								<p class="gc-copy text-[var(--gc-ink-3)]">{renderedEntries.length} visible</p>
							</div>

							{#if streamError !== ''}
								<div
									class="mt-4 border border-[var(--gc-warning)] bg-[var(--gc-warning-dim)] px-4 py-3"
								>
									<p class="gc-stamp text-[var(--gc-warning)]">Connection</p>
									<p class="gc-copy mt-2 text-[var(--gc-ink)]">{streamError}</p>
								</div>
							{/if}

							{#if renderedEntries.length === 0}
								<p class="gc-copy mt-4 text-[var(--gc-ink-2)]">
									No log lines match the current filter set.
								</p>
							{:else}
								<div class="mt-4 flex flex-col gap-4">
									{#each renderedEntries as entry (entry.id)}
										<article
											class="border-b border-[var(--gc-border)] pb-4 last:border-b-0 last:pb-0"
										>
											<div class="flex flex-wrap items-center gap-3">
												<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
													{entry.source}
												</span>
												<span
													class={`gc-badge ${
														entry.level === 'error'
															? 'border-[var(--gc-error)] text-[var(--gc-error)]'
															: entry.level === 'warn'
																? 'border-[var(--gc-warning)] text-[var(--gc-warning)]'
																: 'border-[var(--gc-signal)] text-[var(--gc-signal)]'
													}`}
												>
													{entry.level_label}
												</span>
												<span class="gc-machine text-[var(--gc-ink-3)]"
													>{entry.created_at_label}</span
												>
											</div>
											<p class="gc-copy mt-3 text-[var(--gc-ink)]">{entry.message}</p>
											<pre
												class="gc-code mt-3 overflow-x-auto text-[var(--gc-ink-3)]">{entry.raw}</pre>
										</article>
									{/each}
								</div>
							{/if}
						</div>
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Window summary</p>
						<div class="mt-4 grid gap-4">
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Errors</p>
								<p class="gc-value mt-2 text-[1.2rem]">{errorEntries}</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Warnings</p>
								<p class="gc-value mt-2 text-[1.2rem]">{warningEntries}</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Stream</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">{logs.stream_url}</p>
							</div>
						</div>
					</section>
				</div>
			{:else if activeTab === 'filters'}
				<div class="mt-6 grid gap-4 lg:grid-cols-[minmax(0,1.15fr)_minmax(18rem,0.85fr)]">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Filter toolbar</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Shape the current tail window</h2>

						<form method="GET" action="/logs" class="mt-5 grid gap-4 md:grid-cols-2">
							<input type="hidden" name="tab" value="filters" />

							<label class="block">
								<span class="gc-stamp text-[var(--gc-ink-3)]">Search logs</span>
								<input
									type="text"
									name="q"
									value={logs.filters.query}
									placeholder="panic method=GET"
									class="gc-control mt-2 w-full"
								/>
							</label>

							<label class="block">
								<span class="gc-stamp text-[var(--gc-ink-3)]">Level</span>
								<select name="level" class="gc-control mt-2 w-full" value={logs.filters.level}>
									<option value="all">All levels</option>
									<option value="info">Info</option>
									<option value="warn">Warn</option>
									<option value="error">Error</option>
								</select>
							</label>

							<label class="block">
								<span class="gc-stamp text-[var(--gc-ink-3)]">Source</span>
								<select name="source" class="gc-control mt-2 w-full" value={logs.filters.source}>
									{#each filterSourceOptions as source (source)}
										<option value={source}>{source === 'all' ? 'All sources' : source}</option>
									{/each}
								</select>
							</label>

							<label class="block">
								<span class="gc-stamp text-[var(--gc-ink-3)]">Window Size</span>
								<select
									name="limit"
									class="gc-control mt-2 w-full"
									value={String(logs.filters.limit)}
								>
									{#each limitOptions as limit (limit)}
										<option value={String(limit)}>{limit} lines</option>
									{/each}
								</select>
							</label>

							<div class="flex flex-wrap gap-3 md:col-span-2">
								<button type="submit" class="gc-action gc-action-solid px-4 py-2"
									>Apply Filters</button
								>
								<a href={resolve('/logs?tab=filters')} class="gc-action px-4 py-2">
									Clear Filters
								</a>
							</div>
						</form>
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Current shape</p>
						<div class="mt-4 grid gap-4">
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Matching lines</p>
								<p class="gc-value mt-2 text-[1.2rem]">{renderedEntries.length}</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Current query</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">
									{logs.filters.query === '' ? 'No text filter applied.' : logs.filters.query}
								</p>
							</div>
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Stream path</p>
								<p class="gc-copy mt-2 break-all text-[var(--gc-ink)]">{logs.stream_url}</p>
							</div>
						</div>
					</section>
				</div>
			{:else}
				<div class="mt-6 grid gap-4 lg:grid-cols-[minmax(0,1.1fr)_minmax(18rem,0.9fr)]">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Export buffer</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
							Capture the current investigation window
						</h2>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">{exportCountLabel}</p>

						<div class="mt-5 flex flex-wrap gap-3">
							<button
								type="button"
								onclick={downloadJSONL}
								class="gc-action gc-action-solid px-4 py-2"
							>
								Download JSONL
							</button>
							<button type="button" onclick={copyJSONL} class="gc-action px-4 py-2">
								Copy JSONL
							</button>
						</div>

						{#if exportNotice !== ''}
							<p class="gc-copy mt-4 text-[var(--gc-signal)]">{exportNotice}</p>
						{/if}

						<pre
							class="gc-code mt-5 max-h-[26rem] overflow-auto border border-[var(--gc-border)] bg-[var(--gc-canvas)] px-4 py-4">{exportJSONL ===
							''
								? 'No lines ready.'
								: exportJSONL}</pre>
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Handoff note</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							The exported buffer preserves the current filter set and the same line order you are
							seeing in Live Tail.
						</p>
					</section>
				</div>
			{/if}
		{/if}
	</div>
</div>
