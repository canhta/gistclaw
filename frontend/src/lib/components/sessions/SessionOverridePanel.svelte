<script lang="ts">
	import SessionDeliveryBoard from '$lib/components/sessions/SessionDeliveryBoard.svelte';
	import type { CreateRouteInput } from '$lib/conversations/actions';
	import type {
		ConversationDeliveryQueueResponse,
		ConversationDetailResponse
	} from '$lib/types/api';
	import type { RecoverRuntimeHealthResponse } from '$lib/types/api';

	let {
		detail,
		deliveryQueue,
		runtimeConnectors = [],
		onBindRoute,
		onDeactivateRoute,
		onSendRouteMessage,
		onRetryDelivery,
		bindingRoute = false,
		deactivatingRoute = false,
		sendingRouteMessage = false,
		retryingDeliveryID = null,
		notice = '',
		error = ''
	}: {
		detail: ConversationDetailResponse;
		deliveryQueue: ConversationDeliveryQueueResponse;
		runtimeConnectors?: RecoverRuntimeHealthResponse[];
		onBindRoute?: (input: CreateRouteInput) => void;
		onDeactivateRoute?: () => void;
		onSendRouteMessage?: (body: string) => Promise<boolean>;
		onRetryDelivery?: (deliveryID: string) => void;
		bindingRoute?: boolean;
		deactivatingRoute?: boolean;
		sendingRouteMessage?: boolean;
		retryingDeliveryID?: string | null;
		notice?: string;
		error?: string;
	} = $props();

	let connectorID = $state('');
	let externalID = $state('');
	let threadID = $state('');
	let accountID = $state('');
	let routeMessage = $state('');

	const canBind = $derived(!bindingRoute && connectorID.trim() !== '' && externalID.trim() !== '');
	const canSendRouteMessage = $derived(
		!sendingRouteMessage && routeMessage.trim() !== '' && Boolean(detail.route)
	);

	$effect(() => {
		if (connectorID === '' && runtimeConnectors[0]?.connector_id) {
			connectorID = runtimeConnectors[0].connector_id;
		}
	});

	function handleBindSubmit(event: SubmitEvent): void {
		event.preventDefault();
		if (!canBind) {
			return;
		}

		onBindRoute?.({
			sessionID: detail.session.id,
			connectorID: connectorID.trim(),
			externalID: externalID.trim(),
			threadID: threadID.trim() || undefined,
			accountID: accountID.trim() || undefined
		});
	}

	async function handleRouteSendSubmit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (!canSendRouteMessage) {
			return;
		}

		const sent = await onSendRouteMessage?.(routeMessage.trim());
		if (sent) {
			routeMessage = '';
		}
	}
</script>

<div class="grid flex-1 gap-5 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,0.9fr)]">
	<section class="gc-panel px-5 py-5">
		<p class="gc-stamp text-[var(--gc-warning)]">OVERRIDES</p>
		<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Manage route and delivery overrides</h2>
		<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
			Use the selected session as the unit of control. Route state comes from the live conversation
			detail seam, and delivery recovery actions stay journal-backed.
		</p>
		{#if notice}
			<p class="gc-copy mt-4 text-[var(--gc-primary)]">{notice}</p>
		{/if}
		{#if error}
			<p class="gc-copy mt-2 text-[var(--gc-error)]">{error}</p>
		{/if}

		<div class="mt-5 grid gap-4 lg:grid-cols-2">
			<div class="border border-[var(--gc-border)] px-4 py-4">
				<p class="gc-stamp text-[var(--gc-ink-3)]">Selected Session</p>
				<p class="gc-copy mt-2 text-[var(--gc-ink)]">{detail.session.id}</p>
				<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
					{detail.session.agent_id} · {detail.session.role_label}
				</p>
			</div>

			<div class="border border-[var(--gc-border)] px-4 py-4">
				<p class="gc-stamp text-[var(--gc-ink-3)]">Active Run</p>
				<p class="gc-copy mt-2 text-[var(--gc-ink)]">{detail.active_run_id ?? 'None'}</p>
				<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
					Retry or route actions reload this detail after the journal settles.
				</p>
			</div>
		</div>

		{#if detail.route}
			<div class="mt-5 border border-[var(--gc-border)] px-4 py-4">
				<div class="flex items-start justify-between gap-3">
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">Route</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">{detail.route.id}</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
							{detail.route.connector_id} · {detail.route.status_label}
						</p>
					</div>
					<button
						type="button"
						class="gc-action gc-action-warning px-4 py-2 disabled:opacity-50"
						disabled={deactivatingRoute}
						onclick={() => onDeactivateRoute?.()}
					>
						{deactivatingRoute ? 'Deactivating…' : 'Deactivate route'}
					</button>
				</div>

				<div class="mt-4 grid gap-3 lg:grid-cols-2">
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">External ID</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">{detail.route.external_id}</p>
					</div>
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">Thread ID</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">{detail.route.thread_id}</p>
					</div>
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">Created</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">{detail.route.created_at_label}</p>
					</div>
					<div>
						<p class="gc-stamp text-[var(--gc-ink-3)]">Deactivated</p>
						<p class="gc-copy mt-2 text-[var(--gc-ink)]">
							{detail.route.deactivated_label ?? 'Still active'}
						</p>
					</div>
				</div>

				<form class="mt-5 border-t border-[var(--gc-border)] pt-5" onsubmit={handleRouteSendSubmit}>
					<p class="gc-stamp text-[var(--gc-ink-3)]">Send route message</p>
					<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
						Wake the bound session with a manual operator message.
					</p>
					<label class="mt-4 flex flex-col gap-2">
						<span class="gc-copy text-[var(--gc-ink-2)]">Message body</span>
						<textarea
							bind:value={routeMessage}
							rows={3}
							class="gc-control min-h-[6rem] resize-y py-3"
							placeholder="Ask the bound session what changed."
						></textarea>
					</label>
					<div class="mt-4 flex justify-end">
						<button
							type="submit"
							class="gc-action gc-action-solid min-w-[10rem] justify-center disabled:opacity-50"
							disabled={!canSendRouteMessage}
						>
							{sendingRouteMessage ? 'Sending…' : 'Send message'}
						</button>
					</div>
				</form>
			</div>
		{:else}
			<div class="mt-5 border border-dashed border-[var(--gc-border)] px-4 py-5">
				<p class="gc-copy text-[var(--gc-ink)]">No active route is bound to this session.</p>
				<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
					Bind a live connector target to let inbound work and delivery recovery stay attached to
					this session.
				</p>

				<form class="mt-5 grid gap-4 lg:grid-cols-2" onsubmit={handleBindSubmit}>
					<label class="flex flex-col gap-2">
						<span class="gc-copy text-[var(--gc-ink-2)]">Connector</span>
						<select bind:value={connectorID} class="gc-control min-h-[2.75rem]">
							<option value="" disabled>Select connector</option>
							{#each runtimeConnectors as connector (connector.connector_id)}
								<option value={connector.connector_id}>{connector.connector_id}</option>
							{/each}
						</select>
					</label>

					<label class="flex flex-col gap-2">
						<span class="gc-copy text-[var(--gc-ink-2)]">External ID</span>
						<input
							bind:value={externalID}
							class="gc-control min-h-[2.75rem]"
							placeholder="Chat ID, phone, or user handle"
						/>
					</label>

					<label class="flex flex-col gap-2">
						<span class="gc-copy text-[var(--gc-ink-2)]">Thread ID</span>
						<input
							bind:value={threadID}
							class="gc-control min-h-[2.75rem]"
							placeholder="Optional thread or topic"
						/>
					</label>

					<label class="flex flex-col gap-2">
						<span class="gc-copy text-[var(--gc-ink-2)]">Account override</span>
						<input
							bind:value={accountID}
							class="gc-control min-h-[2.75rem]"
							placeholder="Optional account ID"
						/>
					</label>

					<div class="flex items-end justify-end lg:col-span-2">
						<button
							type="submit"
							class="gc-action gc-action-solid min-w-[10rem] justify-center disabled:opacity-50"
							disabled={!canBind}
						>
							{bindingRoute ? 'Binding…' : 'Bind route'}
						</button>
					</div>
				</form>
			</div>
		{/if}
	</section>

	<section class="gc-panel-soft flex min-h-0 flex-col overflow-hidden px-0 py-0">
		<SessionDeliveryBoard {detail} {deliveryQueue} {retryingDeliveryID} {onRetryDelivery} />
	</section>
</div>
