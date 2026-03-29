<script lang="ts">
	import { goto } from '$app/navigation';
	import SessionDetail from '$lib/components/sessions/SessionDetail.svelte';
	import SessionRow from '$lib/components/sessions/SessionRow.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import { loadConversationDetail } from '$lib/conversations/load';
	import type { ConversationDetailResponse } from '$lib/types/api';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = [
		{ id: 'list', label: 'List' },
		{ id: 'overrides', label: 'Overrides' },
		{ id: 'history', label: 'History' }
	];

	let activeTab = $state('list');
	let selectedID = $state<string | null>(null);
	let detail = $state<ConversationDetailResponse | null>(null);
	let detailLoading = $state(false);
	let detailError = $state('');
	let searchQuery = $state('');

	const sessions = $derived(data.sessions?.items ?? []);
	const filteredSessions = $derived.by(() => {
		const query = searchQuery.trim().toLowerCase();
		if (!query) {
			return sessions;
		}

		return sessions.filter((session) =>
			[
				session.id,
				session.conversation_id,
				session.agent_id,
				session.role_label,
				session.status_label
			].some((value) => value.toLowerCase().includes(query))
		);
	});

	$effect(() => {
		if (!searchQuery) {
			searchQuery = new URLSearchParams(data.currentSearch).get('q') ?? '';
		}
	});

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
		<SectionTabs {tabs} bind:activeTab />
	</div>

	<div class="flex min-h-0 flex-1 overflow-hidden">
		{#if activeTab === 'list'}
			<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
				<div
					class="shrink-0 border-b border-b-[1px] border-[var(--gc-border)] bg-[var(--gc-surface-raised)] px-4 py-3"
				>
					<div class="flex items-center justify-between gap-4">
						<label class="flex min-w-0 flex-1 flex-col gap-2">
							<span class="gc-stamp text-[var(--gc-ink-3)]">Search sessions</span>
							<input
								bind:value={searchQuery}
								type="search"
								placeholder="Search sessions"
								class="gc-control min-h-[2.5rem]"
							/>
						</label>
						<div class="shrink-0 text-right">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Loaded</p>
							<p class="gc-machine mt-1 text-[var(--gc-ink-2)]">
								{filteredSessions.length} / {sessions.length}
							</p>
						</div>
					</div>
				</div>

				<div class="min-h-0 flex-1 overflow-auto">
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
							{#each filteredSessions as session (session.id)}
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
					{:else if filteredSessions.length === 0}
						<div class="flex items-center justify-center p-10">
							<div class="text-center">
								<p class="gc-stamp text-[var(--gc-ink-3)]">FILTERS</p>
								<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No matches</p>
								<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
									No sessions match the current search.
								</p>
							</div>
						</div>
					{/if}
				</div>
			</div>

			{#if selectedID}
				<div
					class="flex min-h-0 w-[24rem] shrink-0 flex-col overflow-hidden border-l border-l-[1.5px] border-[var(--gc-border)] bg-[var(--gc-surface)]"
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
						Detailed session history is not connected to a backend yet.
					</p>
				</div>
			</div>
		{/if}
	</div>
</div>
