<script lang="ts">
	import { goto } from '$app/navigation';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SessionDetail from '$lib/components/sessions/SessionDetail.svelte';
	import SessionRow from '$lib/components/sessions/SessionRow.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import { retryConversationDelivery } from '$lib/conversations/actions';
	import { loadConversationDetail } from '$lib/conversations/load';
	import type { ConversationDetailResponse } from '$lib/types/api';
	import type { PageData } from './$types';

	type TabID = 'list' | 'overrides' | 'history';

	let { data }: { data: PageData } = $props();

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'list', label: 'List' },
		{ id: 'overrides', label: 'Overrides' },
		{ id: 'history', label: 'History' }
	];

	let activeTabOverride = $state<TabID | null>(null);
	let selectedID = $state<string | null>(null);
	let detail = $state<ConversationDetailResponse | null>(null);
	let detailLoading = $state(false);
	let detailError = $state('');
	let retryingDeliveryID = $state<string | null>(null);
	let detailRetryNotice = $state('');
	let detailRetryError = $state('');

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'list';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const summary = $derived(data.sessions.summary);
	const filters = $derived(data.sessions.filters);
	const sessions = $derived(data.sessions.items ?? []);
	const paging = $derived(data.sessions.paging);
	const runtimeConnectors = $derived(data.sessions.runtimeConnectors ?? []);
	const history = $derived(data.sessions.history);

	function isTabID(value: string | null): value is TabID {
		return value === 'list' || value === 'overrides' || value === 'history';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	async function selectSession(id: string): Promise<void> {
		if (selectedID === id) {
			selectedID = null;
			detail = null;
			detailError = '';
			detailRetryNotice = '';
			detailRetryError = '';
			return;
		}

		selectedID = id;
		detailLoading = true;
		detailError = '';
		detailRetryNotice = '';
		detailRetryError = '';

		try {
			detail = await loadConversationDetail(globalThis.fetch.bind(globalThis), id);
		} catch {
			detail = null;
			detailError = 'Failed to load session detail.';
		} finally {
			detailLoading = false;
		}
	}

	function openChat(sessionID: string): void {
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		void goto(`/chat?session=${encodeURIComponent(sessionID)}`);
	}

	function clearFilters(): void {
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		void goto('/sessions?tab=list');
	}

	function clearHistoryFilters(): void {
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		void goto('/sessions?tab=history');
	}

	async function handleRetryDelivery(deliveryID: string): Promise<void> {
		if (!detail) {
			return;
		}

		retryingDeliveryID = deliveryID;
		detailRetryNotice = '';
		detailRetryError = '';

		try {
			await retryConversationDelivery(
				globalThis.fetch.bind(globalThis),
				detail.session.id,
				deliveryID
			);
			detailRetryNotice = 'Delivery requeued.';

			try {
				detail = await loadConversationDetail(globalThis.fetch.bind(globalThis), detail.session.id);
			} catch {
				detailRetryNotice = 'Delivery requeued. Refresh the session to see the latest state.';
			}
		} catch (err) {
			detailRetryError =
				err instanceof Error && err.message.trim() !== ''
					? err.message
					: 'Failed to retry delivery.';
		} finally {
			retryingDeliveryID = null;
		}
	}
</script>

<svelte:head>
	<title>Sessions | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Sessions</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-hidden px-6 py-6">
		<div class="grid gap-4 xl:grid-cols-3">
			<SurfaceMetricCard
				label="Visible Sessions"
				value={String(summary.session_count ?? 0)}
				detail="Sessions loaded for the active project and current filter set."
			/>
			<SurfaceMetricCard
				label="Connected Lanes"
				value={String(summary.connector_count ?? 0)}
				detail="Connector routes represented in the current session view."
				tone="accent"
			/>
			<SurfaceMetricCard
				label="Terminal Deliveries"
				value={String(summary.terminal_deliveries ?? 0)}
				detail="Outbound deliveries that ended terminally across the current page."
				tone="warning"
			/>
		</div>

		<div class="mt-5 flex min-h-0 flex-1 overflow-hidden">
			{#if activeTab === 'list'}
				<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
					<form
						method="GET"
						action="/sessions"
						class="gc-panel-soft grid gap-4 px-4 py-4 xl:grid-cols-[minmax(0,1.4fr)_repeat(4,minmax(0,0.7fr))_auto_auto]"
					>
						<input type="hidden" name="tab" value="list" />
						<div class="xl:col-span-7">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Filter sessions</p>
						</div>

						<label class="flex min-w-0 flex-col gap-2 xl:col-span-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Search sessions</span>
							<input
								type="search"
								name="q"
								value={filters.query}
								placeholder="Search sessions"
								class="gc-control min-h-[2.75rem]"
							/>
						</label>

						<label class="flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Status</span>
							<select name="status" class="gc-control min-h-[2.75rem]">
								<option value="" selected={filters.status === ''}>All statuses</option>
								<option value="active" selected={filters.status === 'active'}>Active</option>
								<option value="archived" selected={filters.status === 'archived'}>Archived</option>
							</select>
						</label>

						<label class="flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Role</span>
							<select name="role" class="gc-control min-h-[2.75rem]">
								<option value="" selected={filters.role === ''}>All roles</option>
								<option value="front" selected={filters.role === 'front'}>Lead agent</option>
								<option value="worker" selected={filters.role === 'worker'}>Specialist agent</option
								>
							</select>
						</label>

						<label class="flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Binding</span>
							<select name="binding" class="gc-control min-h-[2.75rem]">
								<option value="" selected={filters.binding === ''}>All bindings</option>
								<option value="bound" selected={filters.binding === 'bound'}>Bound</option>
								<option value="unbound" selected={filters.binding === 'unbound'}>Unbound</option>
							</select>
						</label>

						<label class="flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Connector</span>
							<select name="connector_id" class="gc-control min-h-[2.75rem]">
								<option value="" selected={filters.connector_id === ''}>All connectors</option>
								{#each runtimeConnectors as connector (connector.connector_id)}
									<option
										value={connector.connector_id}
										selected={filters.connector_id === connector.connector_id}
									>
										{connector.connector_id}
									</option>
								{/each}
							</select>
						</label>

						<div class="flex items-end">
							<button type="submit" class="gc-action gc-action-solid min-w-[9rem] justify-center">
								Apply filters
							</button>
						</div>

						<div class="flex items-end">
							<button
								type="button"
								class="gc-action gc-action-accent min-w-[9rem] justify-center"
								onclick={clearFilters}
							>
								Clear filters
							</button>
						</div>
					</form>

					<div class="mt-5 min-h-0 flex-1 overflow-auto">
						<table class="w-full border-collapse">
							<thead class="sticky top-0 bg-[var(--gc-surface)]">
								<tr class="border-b border-b-[1.5px] border-[var(--gc-border-strong)]">
									<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Session</th>
									<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Agent</th>
									<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Role</th>
									<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Status</th>
									<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Updated</th>
								</tr>
							</thead>
							<tbody>
								{#each sessions as session (session.id)}
									<SessionRow
										{session}
										selected={selectedID === session.id}
										onclick={() => void selectSession(session.id)}
									/>
								{/each}
							</tbody>
						</table>

						{#if sessions.length === 0}
							<div class="flex items-center justify-center p-10">
								<div class="text-center">
									<p class="gc-stamp text-[var(--gc-ink-3)]">SESSIONS</p>
									<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No sessions</p>
									<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
										Sessions are created when channels receive messages.
									</p>
								</div>
							</div>
						{/if}
					</div>

					{#if paging.has_prev || paging.has_next}
						<div
							class="mt-5 flex items-center justify-between border-t border-[var(--gc-border)] pt-4"
						>
							<div class="gc-copy text-[var(--gc-ink-2)]">
								Page through the current filtered session set.
							</div>
							<div class="flex gap-3">
								{#if paging.prevHref}
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
									<a href={paging.prevHref} class="gc-action gc-action-accent">Previous Page</a>
								{/if}
								{#if paging.nextHref}
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
									<a href={paging.nextHref} class="gc-action gc-action-solid">Next Page</a>
								{/if}
							</div>
						</div>
					{/if}
				</div>

				{#if selectedID}
					<div
						class="ml-5 flex min-h-0 w-[24rem] shrink-0 flex-col overflow-hidden border-l border-l-[1.5px] border-[var(--gc-border)] bg-[var(--gc-surface)]"
					>
						{#if detailLoading}
							<div class="flex flex-1 items-center justify-center p-8">
								<p class="gc-copy text-[var(--gc-ink-3)]">Loading...</p>
							</div>
						{:else if detailError}
							<div class="px-5 py-4">
								<p class="gc-copy text-[var(--gc-error)]">{detailError}</p>
							</div>
						{:else if detail}
							<SessionDetail
								{detail}
								{retryingDeliveryID}
								retryNotice={detailRetryNotice}
								retryError={detailRetryError}
								onOpenChat={() => openChat(detail!.session.id)}
								onRetryDelivery={(deliveryID) => void handleRetryDelivery(deliveryID)}
							/>
						{/if}
					</div>
				{/if}
			{:else if activeTab === 'overrides'}
				<div class="grid flex-1 gap-5 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,0.9fr)]">
					<section class="gc-panel px-5 py-5">
						<p class="gc-stamp text-[var(--gc-warning)]">OVERRIDES</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
							Session overrides still depend on runtime-owned state.
						</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							GistClaw can inspect a session and jump straight into Chat, but it does not expose a
							browser write path for route or delivery overrides yet. Treat this panel as the
							handoff point into the existing operator surfaces rather than a dead end.
						</p>
						<div class="mt-5 border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Current seam</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								The shipped conversations API is read-only. Routing and delivery policy still live
								in runtime-managed state and connector configuration.
							</p>
						</div>
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Use these surfaces</p>
						<div class="mt-4 space-y-4">
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Chat</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Jump into the active run to inspect intent and current operator context before
									changing policy.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Channels</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Check connector health and delivery pressure before treating a session issue as a
									per-session override problem.
								</p>
							</div>
							<div>
								<p class="gc-panel-title text-[var(--gc-ink)]">Config</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
									Keep durable routing or host-access policy in Config until a dedicated
									`sessions.patch` path exists.
								</p>
							</div>
						</div>
					</section>
				</div>
			{:else}
				<div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">PROJECT HISTORY</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Project history</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							The current history API is project-wide evidence rather than a session-scoped event
							log. It is still useful here: runs, approvals, and delivery outcomes show what
							happened around the conversations you are inspecting.
						</p>
					</section>

					<form
						method="GET"
						action="/sessions"
						class="gc-panel-soft mt-5 grid gap-4 px-4 py-4 xl:grid-cols-[minmax(0,1.5fr)_repeat(3,minmax(0,0.6fr))_auto_auto]"
					>
						<input type="hidden" name="tab" value="history" />
						<div class="xl:col-span-6">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Filter evidence</p>
						</div>

						<label class="flex min-w-0 flex-col gap-2 xl:col-span-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Search evidence</span>
							<input
								type="search"
								name="history_q"
								value={history.filters.query}
								placeholder="Search runs"
								class="gc-control min-h-[2.75rem]"
							/>
						</label>

						<label class="flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Run status</span>
							<select name="history_status" class="gc-control min-h-[2.75rem]">
								<option value="" selected={history.filters.status === ''}>All run states</option>
								<option value="active" selected={history.filters.status === 'active'}>Active</option
								>
								<option value="pending" selected={history.filters.status === 'pending'}
									>Pending</option
								>
								<option
									value="needs_approval"
									selected={history.filters.status === 'needs_approval'}
								>
									Needs approval
								</option>
								<option value="completed" selected={history.filters.status === 'completed'}>
									Completed
								</option>
								<option value="failed" selected={history.filters.status === 'failed'}>Failed</option
								>
								<option value="interrupted" selected={history.filters.status === 'interrupted'}>
									Interrupted
								</option>
							</select>
						</label>

						<label class="flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Scope</span>
							<select name="history_scope" class="gc-control min-h-[2.75rem]">
								<option value="active" selected={history.filters.scope === 'active'}>
									Active project
								</option>
								<option value="all" selected={history.filters.scope === 'all'}>All projects</option>
							</select>
						</label>

						<label class="flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Limit</span>
							<select name="history_limit" class="gc-control min-h-[2.75rem]">
								<option value="10" selected={history.filters.limit === 10}>10</option>
								<option
									value="20"
									selected={history.filters.limit === 0 || history.filters.limit === 20}
								>
									20
								</option>
								<option value="50" selected={history.filters.limit === 50}>50</option>
							</select>
						</label>

						<div class="flex items-end">
							<button type="submit" class="gc-action gc-action-solid min-w-[9rem] justify-center">
								Apply filters
							</button>
						</div>

						<div class="flex items-end">
							<button
								type="button"
								class="gc-action gc-action-accent min-w-[11rem] justify-center"
								onclick={clearHistoryFilters}
							>
								Clear evidence filters
							</button>
						</div>
					</form>

					<div class="gc-panel-soft mt-4 px-5 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Filter scope</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							Run filters only affect the run lane. Approval and delivery evidence remain recent
							project-wide snapshots until the history API exposes scoped evidence filters.
						</p>
					</div>

					<div class="mt-5 grid gap-4 xl:grid-cols-4">
						<SurfaceMetricCard
							label="Runs"
							value={String(history.summary.run_count ?? 0)}
							detail="Run evidence loaded from the shared history feed."
						/>
						<SurfaceMetricCard
							label="Completed"
							value={String(history.summary.completed_runs ?? 0)}
							detail="Runs that finished without operator recovery."
							tone="accent"
						/>
						<SurfaceMetricCard
							label="Recovery"
							value={String(history.summary.recovery_runs ?? 0)}
							detail="Runs that failed, were interrupted, or needed approval."
							tone="warning"
						/>
						<SurfaceMetricCard
							label="Approvals"
							value={String(history.summary.approval_events ?? 0)}
							detail="Resolved approval events captured by the evidence feed."
						/>
					</div>

					<div class="mt-5 grid gap-5 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,0.9fr)]">
						<section class="gc-panel px-5 py-5">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Recent runs</p>
							{#if history.runs.length === 0}
								<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">No run evidence yet.</p>
							{:else}
								<div class="mt-4 flex flex-col gap-4">
									{#each history.runs as run (run.root.id)}
										<div class="border-b border-[var(--gc-border)] pb-4 last:border-b-0 last:pb-0">
											<div class="flex items-start justify-between gap-4">
												<div class="min-w-0">
													<p class="gc-panel-title text-[var(--gc-ink)]">{run.root.objective}</p>
													<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
														{run.root.id} · {run.root.agent_id}
													</p>
												</div>
												<span class="gc-badge border-[var(--gc-warning)] text-[var(--gc-warning)]">
													{run.root.status_label}
												</span>
											</div>
											<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
												{run.blocker_label || run.child_count_label || 'No blockers'}
											</p>
										</div>
									{/each}
								</div>
							{/if}
						</section>

						<section class="gc-panel-soft px-5 py-5">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Approval evidence</p>
							{#if history.approvals.length === 0}
								<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">No resolved approvals yet.</p>
							{:else}
								<div class="mt-4 flex flex-col gap-4">
									{#each history.approvals as approval (approval.id)}
										<div class="border-b border-[var(--gc-border)] pb-4 last:border-b-0 last:pb-0">
											<p class="gc-panel-title text-[var(--gc-ink)]">{approval.tool_name}</p>
											<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
												{approval.status_label} by {approval.resolved_by || 'operator'}
											</p>
											<p class="gc-machine mt-2 text-[var(--gc-ink-4)]">
												run {approval.run_id} · {approval.resolved_at_label}
											</p>
										</div>
									{/each}
								</div>
							{/if}
						</section>
					</div>

					<section class="gc-panel-soft mt-5 px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Delivery outcomes</p>
						{#if history.deliveries.length === 0}
							<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">No delivery evidence yet.</p>
						{:else}
							<div class="mt-4 flex flex-col gap-4">
								{#each history.deliveries as delivery (delivery.id)}
									<div class="border-b border-[var(--gc-border)] pb-4 last:border-b-0 last:pb-0">
										<div class="flex items-start justify-between gap-4">
											<div>
												<p class="gc-panel-title text-[var(--gc-ink)]">{delivery.connector_id}</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
													{delivery.message_preview}
												</p>
											</div>
											<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
												{delivery.status_label}
											</span>
										</div>
										<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
											{delivery.attempts_label} · {delivery.last_attempt_at_label}
										</p>
									</div>
								{/each}
							</div>
						{/if}
					</section>
				</div>
			{/if}
		</div>
	</div>
</div>
