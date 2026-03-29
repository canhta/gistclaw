<script lang="ts">
	import type { ConversationDetailResponse } from '$lib/types/api';

	let {
		detail,
		onOpenChat,
		onRetryDelivery,
		retryingDeliveryID = null,
		retryNotice = '',
		retryError = ''
	}: {
		detail: ConversationDetailResponse;
		onOpenChat?: () => void;
		onRetryDelivery?: (deliveryID: string) => void;
		retryingDeliveryID?: string | null;
		retryNotice?: string;
		retryError?: string;
	} = $props();
</script>

<div class="flex h-full flex-col overflow-hidden">
	<div class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border)] px-5 py-4">
		<div class="flex items-start justify-between gap-4">
			<div class="min-w-0">
				<p class="gc-stamp text-[var(--gc-ink-3)]">SESSION</p>
				<p class="gc-panel-title mt-1 truncate text-[var(--gc-ink)]">{detail.session.id}</p>
			</div>
			<button
				type="button"
				onclick={onOpenChat}
				class="gc-badge border-[var(--gc-primary)] text-[var(--gc-primary)]"
			>
				Open Chat
			</button>
		</div>
		<div class="mt-2 flex items-center gap-3">
			<span class="gc-copy text-[var(--gc-ink-3)]">{detail.session.agent_id}</span>
			<span class="gc-copy text-[var(--gc-ink-4)]">·</span>
			<span class="gc-copy text-[var(--gc-ink-3)]">{detail.session.role_label}</span>
			<span class="gc-copy text-[var(--gc-ink-4)]">·</span>
			<span class="gc-copy text-[var(--gc-ink-3)]">{detail.session.status_label}</span>
		</div>
	</div>

	<div class="flex-1 overflow-y-auto">
		{#if detail.messages.length === 0}
			<div class="flex items-center justify-center p-8">
				<p class="gc-copy text-[var(--gc-ink-3)]">No messages</p>
			</div>
		{:else}
			{#each detail.messages as msg, i (i)}
				<div class="border-b border-[var(--gc-border)] px-5 py-3">
					<div class="mb-1 flex items-center gap-3">
						<span class="gc-stamp text-[var(--gc-ink-3)]">{msg.kind_label}</span>
						<span class="gc-copy text-[var(--gc-ink-2)]">{msg.sender_label}</span>
					</div>
					<p class="gc-copy whitespace-pre-wrap text-[var(--gc-ink)]">{msg.body.plain_text}</p>
				</div>
			{/each}
		{/if}
	</div>

	{#if detail.route}
		<div class="shrink-0 border-t border-t-[1.5px] border-[var(--gc-border)] px-5 py-3">
			<p class="gc-stamp text-[var(--gc-ink-3)]">ROUTE</p>
			<div class="mt-1 flex items-center gap-3">
				<span class="gc-copy text-[var(--gc-ink-2)]">{detail.route.connector_id}</span>
				<span class="gc-copy text-[var(--gc-ink-4)]">·</span>
				<span class="gc-copy text-[var(--gc-ink-3)]">{detail.route.status_label}</span>
			</div>
		</div>
	{/if}

	{#if detail.deliveries.length > 0}
		<div class="shrink-0 border-t border-t-[1px] border-[var(--gc-border)] px-5 py-3">
			<div class="flex items-start justify-between gap-3">
				<div>
					<p class="gc-stamp text-[var(--gc-ink-3)]">DELIVERIES</p>
					<p class="gc-copy mt-1 text-[var(--gc-ink-2)]">{detail.deliveries.length} recorded</p>
				</div>
				{#if retryNotice}
					<span class="gc-stamp text-[var(--gc-primary)]">{retryNotice}</span>
				{/if}
			</div>
			{#if retryError}
				<p class="gc-copy mt-3 text-[var(--gc-error)]">{retryError}</p>
			{/if}
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
</div>
