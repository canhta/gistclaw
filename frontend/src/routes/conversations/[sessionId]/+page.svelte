<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import SurfaceActionButton from '$lib/components/common/SurfaceActionButton.svelte';
	import SurfaceEmptyState from '$lib/components/common/SurfaceEmptyState.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import { HTTPError, requestJSON } from '$lib/http/client';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let errorMessage = $state('');
	let sending = $state(false);
	let retryingID = $state('');
	let draft = $state('');
	const activeRunID = $derived(data.conversation.active_run_id ?? '');
	const isBusy = $derived(activeRunID !== '');

	async function sendMessage(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		sending = true;
		errorMessage = '';

		try {
			const response = await requestJSON<{ run_id: string }>(
				fetch,
				`/api/conversations/${data.conversation.session.id}/messages`,
				{
					method: 'POST',
					headers: { 'content-type': 'application/json' },
					body: JSON.stringify({ body: draft })
				}
			);
			await goto(resolve('/work/[runId]', { runId: response.run_id }), {
				invalidateAll: true
			});
		} catch (error) {
			errorMessage = error instanceof HTTPError ? error.message : 'Unable to send this message.';
		} finally {
			sending = false;
		}
	}

	async function retryDelivery(deliveryID: string): Promise<void> {
		retryingID = deliveryID;
		errorMessage = '';

		try {
			await requestJSON(
				fetch,
				`/api/conversations/${data.conversation.session.id}/deliveries/${deliveryID}/retry`,
				{
					method: 'POST'
				}
			);
			await invalidateAll();
		} catch (error) {
			errorMessage = error instanceof HTTPError ? error.message : 'Unable to retry this delivery.';
		} finally {
			retryingID = '';
		}
	}
</script>

<svelte:head>
	<title>{data.conversation.session.agent_id} | Conversations | GistClaw</title>
</svelte:head>

<div class="grid gap-6">
	<section class="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<p class="gc-stamp">{data.conversation.session.role_label}</p>
			<h2 class="gc-section-title mt-3">{data.conversation.session.agent_id}</h2>
			<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">
				Keep the conversation readable, route context explicit, and retries attached to the exact
				external surface that needs repair.
			</p>

			{#if errorMessage}
				<SurfaceMessage
					label="Conversation error"
					message={errorMessage}
					tone="error"
					className="mt-5"
				/>
			{/if}

			{#if isBusy}
				<div class="gc-panel-soft gc-card-warning mt-5 px-4 py-4">
					<p class="gc-stamp text-[var(--gc-orange)]">Conversation busy</p>
					<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
						Active run {activeRunID} is still open. Wait for it to finish before sending.
					</p>
					<a
						class="gc-machine mt-3 inline-flex underline"
						href={resolve('/work/[runId]', { runId: activeRunID })}
					>
						Open active run
					</a>
				</div>
			{/if}

			<form class="mt-6 grid gap-3" onsubmit={sendMessage}>
				<label class="grid gap-2">
					<span class="gc-stamp">Send operator message</span>
					<textarea
						rows="4"
						bind:value={draft}
						class="gc-control"
						placeholder="Ask the session what changed, request a retry, or steer the next reply."
					></textarea>
				</label>
				<SurfaceActionButton type="submit" disabled={sending || draft.trim() === '' || isBusy}>
					{sending ? 'Sending message' : 'Send message'}
				</SurfaceActionButton>
			</form>
		</div>

		<div class="grid gap-4">
			<div class="gc-panel px-4 py-4">
				<p class="gc-stamp">Route authority</p>
				{#if data.conversation.route}
					<p class="gc-value mt-3">{data.conversation.route.connector_id}</p>
					<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
						{data.conversation.route.external_id} · {data.conversation.route.thread_id}
					</p>
				{:else}
					<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
						No external route is bound to this conversation yet.
					</p>
				{/if}
			</div>

			{#if data.conversation.active_run_id}
				<a
					class="gc-panel-soft block px-4 py-4 transition-colors hover:border-[var(--gc-orange)]"
					href={resolve('/work/[runId]', { runId: data.conversation.active_run_id })}
				>
					<p class="gc-stamp">Active run</p>
					<p class="gc-value mt-3">{data.conversation.active_run_id}</p>
					<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
						Open the current orchestration path attached to this conversation.
					</p>
				</a>
			{/if}
		</div>
	</section>

	<section class="grid gap-6 xl:grid-cols-[minmax(0,1.05fr)_minmax(0,0.95fr)]">
		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<div class="flex items-end justify-between gap-4">
				<div>
					<p class="gc-stamp">Conversation timeline</p>
					<h2 class="gc-section-title mt-3">Read the thread before you intervene</h2>
				</div>
				<p class="gc-machine">{data.conversation.messages.length} visible entries</p>
			</div>

			<div class="mt-6 grid gap-4">
				{#each data.conversation.messages as message, index (index)}
					<article class="gc-panel-soft px-4 py-4">
						<div class="flex items-start justify-between gap-4">
							<div>
								<p class="gc-stamp">{message.kind_label}</p>
								<h3 class="gc-panel-title mt-3 text-[1rem]">{message.sender_label}</h3>
							</div>
							{#if message.source_run_id}
								<a
									class="gc-machine underline"
									href={resolve('/work/[runId]', { runId: message.source_run_id })}
								>
									{message.source_run_id}
								</a>
							{/if}
						</div>
						<p class="gc-copy structured-text mt-4 whitespace-pre-wrap text-[var(--gc-ink)]">
							{message.body.plain_text}
						</p>
					</article>
				{/each}
			</div>
		</div>

		<div class="grid gap-6">
			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Outbound deliveries</p>
				<div class="mt-4 grid gap-4">
					{#if data.conversation.deliveries.length === 0}
						<SurfaceEmptyState
							label="No outbound deliveries"
							title="Nothing has been queued recently"
							message="Nothing has been queued from this conversation recently."
						/>
					{:else}
						{#each data.conversation.deliveries as delivery (delivery.id)}
							<article class="gc-panel-soft px-4 py-4">
								<p class="gc-stamp">{delivery.connector_id} · {delivery.status_label}</p>
								<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
									{delivery.message.plain_text}
								</p>
								<p class="gc-machine mt-3">{delivery.attempts_label}</p>
								{#if delivery.status === 'terminal'}
									<SurfaceActionButton
										type="button"
										tone="warning"
										className="mt-4"
										onclick={() => retryDelivery(delivery.id)}
										disabled={retryingID !== '' && retryingID !== delivery.id}
									>
										Retry delivery
									</SurfaceActionButton>
								{/if}
							</article>
						{/each}
					{/if}
				</div>
			</div>

			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Failure evidence</p>
				<div class="mt-4 grid gap-4">
					{#if data.conversation.delivery_failures.length === 0}
						<SurfaceEmptyState
							label="No recorded failures"
							title="No failure receipts are attached"
							message="No failure receipts are attached to this conversation right now."
						/>
					{:else}
						{#each data.conversation.delivery_failures as failure (failure.id)}
							<article class="gc-panel-soft px-4 py-4">
								<p class="gc-stamp">{failure.connector_id} · {failure.event_kind_label}</p>
								<h3 class="gc-panel-title mt-3 text-[1rem]">{failure.chat_id}</h3>
								<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">{failure.error}</p>
								<p class="gc-machine mt-3">{failure.created_at_label}</p>
							</article>
						{/each}
					{/if}
				</div>
			</div>
		</div>
	</section>
</div>
