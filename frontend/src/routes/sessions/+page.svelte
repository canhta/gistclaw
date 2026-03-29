<script lang="ts">
	import { goto } from '$app/navigation';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SessionDetail from '$lib/components/sessions/SessionDetail.svelte';
	import SessionRow from '$lib/components/sessions/SessionRow.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
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
			return;
		}

		selectedID = id;
		detailLoading = true;
		detailError = '';

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
							<SessionDetail {detail} onOpenChat={() => openChat(detail!.session.id)} />
						{/if}
					</div>
				{/if}
			{:else if activeTab === 'overrides'}
				<div class="flex flex-1 items-center justify-center p-10">
					<div class="text-center">
						<p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
						<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Session overrides</p>
						<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
							Session override controls are not connected to a backend yet.
						</p>
					</div>
				</div>
			{:else}
				<div class="flex flex-1 items-center justify-center p-10">
					<div class="text-center">
						<p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
						<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Session history</p>
						<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
							Session history is not connected to a backend yet.
						</p>
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>
