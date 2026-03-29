<script lang="ts">
	import SessionDeliveryBoard from '$lib/components/sessions/SessionDeliveryBoard.svelte';
	import type { ConversationDetailResponse } from '$lib/types/api';

	let {
		detail,
		onDeactivateRoute,
		onRetryDelivery,
		deactivatingRoute = false,
		retryingDeliveryID = null,
		notice = '',
		error = ''
	}: {
		detail: ConversationDetailResponse;
		onDeactivateRoute?: () => void;
		onRetryDelivery?: (deliveryID: string) => void;
		deactivatingRoute?: boolean;
		retryingDeliveryID?: string | null;
		notice?: string;
		error?: string;
	} = $props();
</script>

<div class="grid flex-1 gap-5 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,0.9fr)]">
	<section class="gc-panel px-5 py-5">
		<p class="gc-stamp text-[var(--gc-warning)]">OVERRIDES</p>
		<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Manage route and delivery overrides</h2>
		<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
			Use the selected session as the unit of control. Route state comes from the live conversation
			detail seam, and delivery recovery actions stay journal-backed.
		</p>

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
			</div>
		{:else}
			<div class="mt-5 border border-dashed border-[var(--gc-border)] px-4 py-5">
				<p class="gc-copy text-[var(--gc-ink)]">No active route is bound to this session.</p>
				<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
					When the runtime binds a route, its connector and thread details appear here.
				</p>
			</div>
		{/if}
	</section>

	<section class="gc-panel-soft flex min-h-0 flex-col overflow-hidden px-0 py-0">
		<SessionDeliveryBoard {detail} {retryingDeliveryID} {notice} {error} {onRetryDelivery} />
	</section>
</div>
