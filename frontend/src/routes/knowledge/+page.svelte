<script lang="ts">
	import { resolve } from '$app/paths';
	import { invalidateAll } from '$app/navigation';
	import SurfaceActionButton from '$lib/components/common/SurfaceActionButton.svelte';
	import SurfaceEmptyState from '$lib/components/common/SurfaceEmptyState.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import { HTTPError, requestJSON } from '$lib/http/client';
	import { SvelteURLSearchParams } from 'svelte/reactivity';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let errorMessage = $state('');
	let filterScopeOverride = $state<string | null>(null);
	let filterAgentIDOverride = $state<string | null>(null);
	let filterQueryOverride = $state<string | null>(null);
	let filterLimitOverride = $state<string | null>(null);
	let editsOverride = $state<Record<string, string>>({});
	let savingID = $state('');
	let forgettingID = $state('');

	async function applyFilters(event: SubmitEvent): Promise<void> {
		event.preventDefault();

		const search = new SvelteURLSearchParams();
		if (filterScopeValue().trim() !== '') {
			search.set('scope', filterScopeValue().trim());
		}
		if (filterAgentIDValue().trim() !== '') {
			search.set('agent_id', filterAgentIDValue().trim());
		}
		if (filterQueryValue().trim() !== '') {
			search.set('q', filterQueryValue().trim());
		}
		if (filterLimitValue().trim() !== '') {
			search.set('limit', filterLimitValue().trim());
		}

		window.location.assign(
			`${resolve('/knowledge')}${search.toString() === '' ? '' : `?${search.toString()}`}`
		);
	}

	async function saveEdit(itemID: string): Promise<void> {
		savingID = itemID;
		errorMessage = '';

		try {
			await requestJSON(fetch, `/api/knowledge/${itemID}/edit`, {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ content: knowledgeContent(itemID) })
			});
			editsOverride = {};
			await invalidateAll();
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to save this knowledge item.';
		} finally {
			savingID = '';
		}
	}

	async function forgetItem(itemID: string): Promise<void> {
		forgettingID = itemID;
		errorMessage = '';

		try {
			await requestJSON(fetch, `/api/knowledge/${itemID}/forget`, {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({})
			});
			editsOverride = {};
			await invalidateAll();
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to forget this knowledge item.';
		} finally {
			forgettingID = '';
		}
	}

	function filterScopeValue(): string {
		return filterScopeOverride ?? data.knowledge.filters.scope;
	}

	function filterAgentIDValue(): string {
		return filterAgentIDOverride ?? data.knowledge.filters.agent_id;
	}

	function filterQueryValue(): string {
		return filterQueryOverride ?? data.knowledge.filters.query;
	}

	function filterLimitValue(): string {
		return filterLimitOverride ?? String(data.knowledge.filters.limit);
	}

	function knowledgeContent(itemID: string): string {
		const override = editsOverride[itemID];
		if (override !== undefined) {
			return override;
		}
		const item = data.knowledge.items.find((candidate) => candidate.id === itemID);
		return item?.content ?? '';
	}
</script>

<svelte:head>
	<title>Knowledge | GistClaw</title>
</svelte:head>

<div class="grid gap-6">
	<section class="grid gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<p class="gc-stamp">Saved context</p>
			<h2 class="gc-section-title mt-3">{data.knowledge.headline}</h2>
			<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">
				Save rules and facts in plain language so future work can follow them.
			</p>

			<form class="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4" onsubmit={applyFilters}>
				<label class="grid gap-2">
					<span class="gc-stamp">Scope</span>
					<select
						value={filterScopeValue()}
						onchange={(event) => {
							filterScopeOverride = event.currentTarget.value;
						}}
						class="gc-control"
					>
						<option value="">All scopes</option>
						<option value="local">local</option>
						<option value="team">team</option>
						<option value="global">global</option>
					</select>
				</label>

				<label class="grid gap-2">
					<span class="gc-stamp">Agent</span>
					<input
						value={filterAgentIDValue()}
						oninput={(event) => {
							filterAgentIDOverride = event.currentTarget.value;
						}}
						class="gc-control"
						placeholder="assistant"
					/>
				</label>

				<label class="grid gap-2">
					<span class="gc-stamp">Search</span>
					<input
						value={filterQueryValue()}
						oninput={(event) => {
							filterQueryOverride = event.currentTarget.value;
						}}
						class="gc-control"
						placeholder="repo rule"
					/>
				</label>

				<label class="grid gap-2">
					<span class="gc-stamp">Visible knowledge</span>
					<input
						value={filterLimitValue()}
						oninput={(event) => {
							filterLimitOverride = event.currentTarget.value;
						}}
						class="gc-control"
					/>
				</label>

				<SurfaceActionButton type="submit">Apply filters</SurfaceActionButton>
			</form>

			{#if errorMessage}
				<SurfaceMessage
					label="Knowledge error"
					message={errorMessage}
					tone="error"
					className="mt-5"
				/>
			{/if}
		</div>

		<div class="grid gap-4">
			<div class="gc-panel px-4 py-4">
				<p class="gc-stamp">Knowledge summary</p>
				<div class="mt-4 grid gap-3 sm:grid-cols-2">
					<div>
						<p class="gc-stamp">Visible knowledge</p>
						<p class="gc-value mt-2">{data.knowledge.summary.visible_count}</p>
					</div>
					<div>
						<p class="gc-stamp">Paging state</p>
						<p class="gc-machine mt-2">
							{data.knowledge.paging.has_next || data.knowledge.paging.has_prev
								? 'Paginated'
								: 'Single view'}
						</p>
					</div>
				</div>
			</div>

			<div class="gc-panel-soft px-4 py-4">
				<p class="gc-stamp">What to remember</p>
				<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
					Edit facts in the language you want future work to follow, then remove anything that no
					longer matters.
				</p>
			</div>
		</div>
	</section>

	<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
		<div class="flex flex-wrap items-end justify-between gap-4">
			<div>
				<p class="gc-stamp">Visible knowledge</p>
				<h2 class="gc-section-title mt-3">Review the saved context that shapes later work</h2>
			</div>
			<p class="gc-machine">{data.knowledge.items.length} visible items</p>
		</div>

		{#if data.knowledge.items.length === 0}
			<SurfaceEmptyState
				className="mt-6"
				label="No saved knowledge"
				title="Nothing is shaping future work yet"
				message="Nothing is shaping future work yet for this filter."
			/>
		{:else}
			<div class="mt-6 grid gap-4 xl:grid-cols-2">
				{#each data.knowledge.items as item (item.id)}
					<article class="gc-panel-soft px-4 py-4">
						<div class="flex items-start justify-between gap-4">
							<div>
								<p class="gc-stamp">{item.agent_id} · {item.scope}</p>
								<h3 class="gc-panel-title mt-3 text-[1rem]">
									{item.provenance || 'Captured context'}
								</h3>
							</div>
							<p class="gc-machine">{item.source}</p>
						</div>

						<label class="mt-4 grid gap-2">
							<span class="gc-stamp">Content</span>
							<textarea
								rows="4"
								oninput={(event) => {
									editsOverride = {
										...editsOverride,
										[item.id]: event.currentTarget.value
									};
								}}
								class="gc-control">{knowledgeContent(item.id)}</textarea
							>
						</label>

						<div class="mt-4 grid gap-3 md:grid-cols-2">
							<div class="border-t-2 border-[var(--gc-border)] pt-3">
								<p class="gc-stamp">Updated</p>
								<p class="gc-machine mt-2">{item.updated_at_label}</p>
							</div>
							<div class="border-t-2 border-[var(--gc-border)] pt-3">
								<p class="gc-stamp">Created</p>
								<p class="gc-machine mt-2">{item.created_at_label}</p>
							</div>
						</div>

						<div class="mt-5 flex flex-wrap gap-3">
							<SurfaceActionButton
								type="button"
								tone="solid"
								onclick={() => saveEdit(item.id)}
								disabled={savingID !== '' && savingID !== item.id}
							>
								{savingID === item.id ? 'Saving edit' : 'Save edit'}
							</SurfaceActionButton>

							<SurfaceActionButton
								type="button"
								tone="warning"
								onclick={() => forgetItem(item.id)}
								disabled={forgettingID !== '' && forgettingID !== item.id}
							>
								{forgettingID === item.id ? 'Forgetting item' : 'Forget item'}
							</SurfaceActionButton>
						</div>
					</article>
				{/each}
			</div>
		{/if}
	</section>
</div>
