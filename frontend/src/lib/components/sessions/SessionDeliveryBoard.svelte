<script lang="ts">
	import { goto } from '$app/navigation';
	import type {
		ConversationDeliveryQueueResponse,
		ConversationDetailResponse
	} from '$lib/types/api';

	let {
		detail,
		deliveryQueue,
		onRetryDelivery,
		retryingDeliveryID = null,
		notice = '',
		error = ''
	}: {
		detail: ConversationDetailResponse;
		deliveryQueue?: ConversationDeliveryQueueResponse;
		onRetryDelivery?: (deliveryID: string) => void;
		retryingDeliveryID?: string | null;
		notice?: string;
		error?: string;
	} = $props();

	function clearQueueFilters(): void {
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		void goto(`/sessions?tab=overrides&session=${encodeURIComponent(detail.session.id)}`);
	}
</script>

{#if notice || error}
	<div class="shrink-0 border-t border-t-[1px] border-[var(--gc-border)] px-5 py-3">
		{#if notice}
			<p class="gc-copy text-[var(--gc-primary)]">{notice}</p>
		{/if}
		{#if error}
			<p class="gc-copy mt-2 text-[var(--gc-error)]">{error}</p>
		{/if}
	</div>
{/if}

{#if deliveryQueue}
	<div class="shrink-0 border-t border-t-[1px] border-[var(--gc-border)] px-5 py-3">
		<p class="gc-stamp text-[var(--gc-primary)]">QUEUE</p>
		<p class="gc-panel-title mt-2 text-[var(--gc-ink)]">Session delivery queue</p>
		<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
			Page-backed outbound intents for the selected session.
		</p>
		<form
			method="GET"
			action="/sessions"
			class="mt-4 grid gap-3 border-t border-[var(--gc-border)] pt-4"
		>
			<input type="hidden" name="tab" value="overrides" />
			<input type="hidden" name="session" value={detail.session.id} />
			<p class="gc-stamp text-[var(--gc-ink-3)]">Filter queue</p>
			<label class="flex flex-col gap-2">
				<span class="gc-copy text-[var(--gc-ink-2)]">Search queue</span>
				<input
					type="search"
					name="delivery_q"
					value={deliveryQueue.filters.query}
					placeholder="Search connector, chat, or message"
					class="gc-control min-h-[2.75rem]"
				/>
			</label>
			<div class="grid gap-3 sm:grid-cols-2">
				<label class="flex flex-col gap-2">
					<span class="gc-copy text-[var(--gc-ink-2)]">Queue status</span>
					<select name="delivery_status" class="gc-control min-h-[2.75rem]">
						<option value="" selected={deliveryQueue.filters.status === ''}>All queue states</option
						>
						<option value="pending" selected={deliveryQueue.filters.status === 'pending'}>
							Pending only
						</option>
						<option value="retrying" selected={deliveryQueue.filters.status === 'retrying'}>
							Retrying only
						</option>
						<option value="terminal" selected={deliveryQueue.filters.status === 'terminal'}>
							Terminal only
						</option>
					</select>
				</label>
				<label class="flex flex-col gap-2">
					<span class="gc-copy text-[var(--gc-ink-2)]">Queue limit</span>
					<select name="delivery_limit" class="gc-control min-h-[2.75rem]">
						<option value="10" selected={deliveryQueue.filters.limit === 10}>10 rows</option>
						<option value="25" selected={deliveryQueue.filters.limit === 25}>25 rows</option>
						<option value="50" selected={deliveryQueue.filters.limit === 50}>50 rows</option>
					</select>
				</label>
			</div>
			<div class="flex items-center justify-end gap-3">
				<button type="button" class="gc-action gc-action-accent" onclick={clearQueueFilters}>
					Clear queue filters
				</button>
				<button type="submit" class="gc-action gc-action-solid">Apply queue filters</button>
			</div>
		</form>
		<div class="mt-3 grid gap-3">
			{#each deliveryQueue.items as delivery (delivery.id)}
				<div class="border border-[var(--gc-border)] px-4 py-4">
					<div class="flex items-start justify-between gap-3">
						<div>
							<p class="gc-stamp text-[var(--gc-ink-3)]">Run</p>
							<p class="gc-copy mt-1 text-[var(--gc-ink)]">{delivery.run_id || 'No run linked'}</p>
							<p class="gc-copy mt-3 text-[var(--gc-ink)]">{delivery.connector_id}</p>
							<p class="gc-copy mt-1 text-[var(--gc-ink-2)]">{delivery.chat_id}</p>
						</div>
						<span class="gc-stamp text-[var(--gc-ink-3)]">{delivery.status_label}</span>
					</div>
					<p class="gc-copy mt-3 whitespace-pre-wrap text-[var(--gc-ink)]">
						{delivery.message_preview}
					</p>
					<div class="mt-3 flex items-center justify-between gap-3">
						<p class="gc-copy text-[var(--gc-ink-3)]">{delivery.attempts_label}</p>
						{#if delivery.status === 'terminal'}
							<button
								type="button"
								class="gc-action gc-action-warning px-3 py-2 disabled:opacity-50"
								disabled={retryingDeliveryID === delivery.id}
								onclick={() => onRetryDelivery?.(delivery.id)}
							>
								{retryingDeliveryID === delivery.id ? 'Retrying…' : 'Retry delivery'}
							</button>
						{/if}
					</div>
				</div>
			{/each}
		</div>
		{#if deliveryQueue.items.length === 0}
			<div class="mt-3 border border-dashed border-[var(--gc-border)] px-4 py-4">
				<p class="gc-copy text-[var(--gc-ink)]">No queue rows match the current filters.</p>
			</div>
		{/if}
		{#if deliveryQueue.paging.has_prev || deliveryQueue.paging.has_next}
			<div class="mt-4 flex items-center justify-end gap-3 border-t border-[var(--gc-border)] pt-4">
				{#if deliveryQueue.paging.prevHref}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
					<a href={deliveryQueue.paging.prevHref} class="gc-action gc-action-accent"
						>Previous queue page</a
					>
				{/if}
				{#if deliveryQueue.paging.nextHref}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
					<a href={deliveryQueue.paging.nextHref} class="gc-action gc-action-solid"
						>Next queue page</a
					>
				{/if}
			</div>
		{/if}
	</div>
{/if}

{#if detail.deliveries.length > 0}
	<div class="shrink-0 border-t border-t-[1px] border-[var(--gc-border)] px-5 py-3">
		<div class="flex items-start justify-between gap-3">
			<div>
				<p class="gc-stamp text-[var(--gc-ink-3)]">DELIVERIES</p>
				<p class="gc-copy mt-1 text-[var(--gc-ink-2)]">{detail.deliveries.length} recorded</p>
			</div>
		</div>
		<div class="mt-3 grid gap-3">
			{#each detail.deliveries as delivery (delivery.id)}
				<div class="border border-[var(--gc-border)] px-4 py-4">
					<div class="flex items-start justify-between gap-3">
						<div>
							<p class="gc-copy text-[var(--gc-ink)]">{delivery.connector_id}</p>
							<p class="gc-copy mt-1 text-[var(--gc-ink-2)]">{delivery.chat_id}</p>
						</div>
						<span class="gc-stamp text-[var(--gc-ink-3)]">{delivery.status_label}</span>
					</div>
					<p class="gc-copy mt-3 whitespace-pre-wrap text-[var(--gc-ink)]">
						{delivery.message.plain_text}
					</p>
					<div class="mt-3 flex items-center justify-between gap-3">
						<p class="gc-copy text-[var(--gc-ink-3)]">{delivery.attempts_label}</p>
						{#if delivery.status === 'terminal'}
							<button
								type="button"
								class="gc-action gc-action-warning px-3 py-2 disabled:opacity-50"
								disabled={retryingDeliveryID === delivery.id}
								onclick={() => onRetryDelivery?.(delivery.id)}
							>
								{retryingDeliveryID === delivery.id ? 'Retrying…' : 'Retry delivery'}
							</button>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	</div>
{/if}

{#if detail.delivery_failures.length > 0}
	<div class="shrink-0 border-t border-t-[1px] border-[var(--gc-border)] px-5 py-3">
		<p class="gc-stamp text-[var(--gc-warning)]">DELIVERY FAILURES</p>
		<p class="gc-panel-title mt-2 text-[var(--gc-ink)]">Delivery failures</p>
		<div class="mt-3 grid gap-3">
			{#each detail.delivery_failures as failure (failure.id)}
				<div class="border border-[var(--gc-border)] px-4 py-4">
					<div class="flex items-start justify-between gap-3">
						<div>
							<p class="gc-copy text-[var(--gc-ink)]">{failure.event_kind_label}</p>
							<p class="gc-copy mt-1 text-[var(--gc-ink-2)]">
								{failure.connector_id} · {failure.chat_id}
							</p>
						</div>
						<span class="gc-stamp text-[var(--gc-ink-3)]">{failure.created_at_label}</span>
					</div>
					<p class="gc-copy mt-3 whitespace-pre-wrap text-[var(--gc-error)]">{failure.error}</p>
				</div>
			{/each}
		</div>
	</div>
{/if}
